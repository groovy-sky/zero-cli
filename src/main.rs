mod sandbox;

use anyhow::Result;
use sandbox::Sandbox;
use serde_json::{json, Value};
use std::sync::Arc;
use tokio::io::{AsyncBufReadExt, AsyncWriteExt, BufReader};

// ---------------------------------------------------------------------------
// Newline-delimited JSON stdio transport (MCP stdio spec)
// ---------------------------------------------------------------------------

/// Read one newline-terminated JSON-RPC message from stdin.
/// Returns `None` on clean EOF.
async fn read_message(reader: &mut BufReader<tokio::io::Stdin>) -> Result<Option<Value>> {
    let mut line = String::new();
    let n = reader.read_line(&mut line).await?;
    if n == 0 {
        return Ok(None); // EOF — client closed connection
    }
    let trimmed = line.trim();
    if trimmed.is_empty() {
        return Ok(None);
    }
    let value: Value = serde_json::from_str(trimmed)?;
    Ok(Some(value))
}

/// Write one newline-terminated JSON-RPC message to stdout.
async fn write_message(stdout: &mut tokio::io::Stdout, msg: Value) -> Result<()> {
    let mut json = serde_json::to_string(&msg)?;
    json.push('\n');
    stdout.write_all(json.as_bytes()).await?;
    stdout.flush().await?;
    Ok(())
}

fn make_error(id: Option<Value>, code: i32, message: impl Into<String>) -> Value {
    json!({
        "jsonrpc": "2.0",
        "id": id.unwrap_or(Value::Null),
        "error": { "code": code, "message": message.into() }
    })
}

// ---------------------------------------------------------------------------
// Entry point
// ---------------------------------------------------------------------------

#[tokio::main]
async fn main() -> Result<()> {
    // All diagnostic output goes to stderr so stdout stays clean for JSON-RPC.
    tracing_subscriber::fmt()
        .with_writer(std::io::stderr)
        .with_ansi(false)
        .init();

    let wasm_bytes = include_bytes!("../wasm/uutils.wasm");
    let sandbox = Arc::new(Sandbox::new(wasm_bytes)?);

    let mut reader = BufReader::new(tokio::io::stdin());
    let mut stdout = tokio::io::stdout();

    loop {
        let msg = match read_message(&mut reader).await {
            Ok(Some(m)) => m,
            Ok(None) => break, // clean EOF
            Err(e) => {
                tracing::error!("read error: {e}");
                break;
            }
        };

        // JSON-RPC 2.0: notifications have no id (or null id) — do not respond.
        let id = msg.get("id").cloned().filter(|v| !v.is_null());
        let method = msg
            .get("method")
            .and_then(|m| m.as_str())
            .unwrap_or("")
            .to_owned();
        let params = msg.get("params").cloned().unwrap_or(json!({}));

        tracing::debug!(method = %method, is_request = id.is_some(), "received");

        if id.is_none() {
            // Notification — only act on "exit".
            if method == "exit" {
                break;
            }
            continue;
        }

        let response = match method.as_str() {
            // ----------------------------------------------------------------
            // Lifecycle
            // ----------------------------------------------------------------
            "initialize" => {
                json!({
                    "jsonrpc": "2.0",
                    "id": id,
                    "result": {
                        "protocolVersion": "2024-11-05",
                        "serverInfo": {
                            "name": "wasm-shell-mcp",
                            "version": env!("CARGO_PKG_VERSION")
                        },
                        "capabilities": {
                            "tools": {}
                        }
                    }
                })
            }

            "shutdown" => {
                json!({ "jsonrpc": "2.0", "id": id, "result": null })
            }

            "ping" => {
                json!({ "jsonrpc": "2.0", "id": id, "result": {} })
            }

            // ----------------------------------------------------------------
            // Tools
            // ----------------------------------------------------------------
            "tools/list" => {
                json!({
                    "jsonrpc": "2.0",
                    "id": id,
                    "result": {
                        "tools": [{
                            "name": "shell_run",
                            "description": "Run a coreutils command (ls, grep, cat, …) inside a WASI sandbox. Only directories listed in allowed_paths are visible. No network access.",
                            "inputSchema": {
                                "type": "object",
                                "properties": {
                                    "command": {
                                        "type": "string",
                                        "description": "Coreutils applet name, e.g. \"ls\", \"grep\", \"cat\""
                                    },
                                    "args": {
                                        "type": "array",
                                        "items": { "type": "string" },
                                        "description": "Arguments forwarded verbatim to the applet",
                                        "default": []
                                    },
                                    "stdin": {
                                        "type": "string",
                                        "description": "Data piped to stdin",
                                        "default": ""
                                    },
                                    "allowed_paths": {
                                        "type": "array",
                                        "items": { "type": "string" },
                                        "description": "Host directories exposed inside the sandbox (read-write)",
                                        "default": []
                                    }
                                },
                                "required": ["command"]
                            }
                        }]
                    }
                })
            }

            "tools/call" => {
                let tool_name = params
                    .get("name")
                    .and_then(|n| n.as_str())
                    .unwrap_or("");
                let args = params.get("arguments").cloned().unwrap_or(json!({}));

                match tool_name {
                    "shell_run" => {
                        let command = args
                            .get("command")
                            .and_then(|c| c.as_str())
                            .unwrap_or("")
                            .to_owned();

                        if command.is_empty() {
                            make_error(id, -32602, "\"command\" is required")
                        } else {
                            let cmd_args: Vec<String> = args
                                .get("args")
                                .and_then(|a| a.as_array())
                                .map(|a| {
                                    a.iter()
                                        .filter_map(|v| v.as_str().map(String::from))
                                        .collect()
                                })
                                .unwrap_or_default();

                            let stdin_bytes = args
                                .get("stdin")
                                .and_then(|s| s.as_str())
                                .unwrap_or("")
                                .as_bytes()
                                .to_vec();

                            let allowed_paths: Vec<String> = args
                                .get("allowed_paths")
                                .and_then(|a| a.as_array())
                                .map(|a| {
                                    a.iter()
                                        .filter_map(|v| v.as_str().map(String::from))
                                        .collect()
                                })
                                .unwrap_or_default();

                            let sandbox = Arc::clone(&sandbox);
                            // Wasmtime is synchronous and blocks internally —
                            // must run on a dedicated blocking thread.
                            let run_result = tokio::task::spawn_blocking(move || {
                                sandbox.run(&command, &cmd_args, &stdin_bytes, &allowed_paths)
                            })
                            .await;

                            match run_result {
                                Ok(Ok(out)) => {
                                    json!({
                                        "jsonrpc": "2.0",
                                        "id": id,
                                        "result": {
                                            "stdout": out.stdout,
                                            "stderr": out.stderr,
                                            "exit_code": out.exit_code,
                                        }
                                    })
                                }
                                Ok(Err(e)) => make_error(id, -32000, e.to_string()),
                                Err(e) => make_error(id, -32000, format!("task panicked: {e}")),
                            }
                        }
                    }
                    _ => make_error(id, -32601, format!("unknown tool: {tool_name}")),
                }
            }

            _ => make_error(id, -32601, format!("method not found: {method}")),
        };

        if let Err(e) = write_message(&mut stdout, response).await {
            tracing::error!("write error: {e}");
            break;
        }
    }

    Ok(())
}
