# wasm-shell-mcp

wasm-shell-mcp is an MCP server that runs coreutils applets inside a Wasmtime
WASI sandbox.

The host never spawns a system shell. Every call executes a wasm-compiled
uutils/coreutils applet with explicit capabilities.

## Security Model

Each call is isolated with two guarantees:

- Filesystem: only directories listed in allowed_paths are visible.
- Network: no socket capability is exposed to the guest.

## Tool API

### shell_run

Run one coreutils applet in the sandbox.

| Parameter | Type | Required | Description |
|---|---|---|---|
| command | string | yes | Applet name, for example ls, cat, sort |
| args | string[] | no | Arguments passed verbatim |
| stdin | string | no | Text piped to stdin |
| allowed_paths | string[] | no | Host directories mounted into the sandbox |

Response shape:

```json
{ "stdout": "...", "stderr": "...", "exit_code": 0 }
```

## Available Commands

The default build profile focuses on practical editor and CI workflows.

- File and directory ops: ls, cp, mv, rm, mkdir, touch
- File reading and inspection: cat, head, nl, wc, pwd
- Text shaping and analysis: sort, uniq, tr, tee
- Environment and system context: env, printenv, uname, whoami
- Script helpers: test, printf, mktemp, echo
- Integrity checks: sha256sum

Current default command set:

ls, cat, cp, mv, rm, mkdir, touch, pwd, head, nl, wc, sort, uniq, tr, tee,
sha256sum, env, printenv, uname, whoami, test, printf, mktemp, echo

## Architecture

```text
AI client
    | MCP stdio (JSON-RPC)
    v
wasm-shell-mcp (Rust)
    |- MCP handler
    \- Sandbox executor (wasmtime + WASIp1)
            \- uutils.wasm (wasm32-wasip1)
```

uutils.wasm is embedded at compile time with include_bytes. The module is
compiled once at startup. Each tool call uses a fresh Store and in-memory
stdio pipes.

## Build

### Prerequisites

- Rust 1.85 or newer
- wasm target: rustup target add wasm32-wasip1
- For cross-compilation: required rust targets and linkers/toolchains

### Common targets

```sh
make         # build wasm and cross-platform binaries
make wasm    # build wasm/uutils.wasm only
make build   # cross-compile server binaries only
make clean   # remove generated artifacts
```

Binaries are written to:

bin/wasm-shell-mcp-<os>-<arch>[.exe]

The build step also prints a summary of the most useful VS Code terminal
workflow commands included in the wasm applet set.

### Dev build (current platform)

```sh
cargo build
```

## Usage

The server is stdio-based JSON-RPC. Configure it in your MCP host.

### GitHub Copilot (VS Code)

Workspace config example in .vscode/mcp.json:

```json
{
  "servers": {
    "wasm-shell": {
      "type": "stdio",
      "command": "./bin/wasm-shell-mcp-linux-amd64"
    }
  }
}
```

### Continue (VS Code)

Create .continue/mcpServers/wasm-shell-mcp.yaml:

```yaml
name: WASM Shell MCP server
version: 0.0.1
schema: v1
mcpServers:
  - name: wasm-shell
    command: ./bin/wasm-shell-mcp-linux-amd64
```

### Claude Desktop

macOS config path:
~/Library/Application Support/Claude/claude_desktop_config.json

Windows config path:
%APPDATA%\Claude\claude_desktop_config.json

Example:

```json
{
  "mcpServers": {
    "wasm-shell": {
      "command": "C:\\path\\to\\wasm-shell-mcp-windows-amd64.exe"
    }
  }
}
```

## Project Layout

```text
zero-cli/
|- Cargo.toml
|- Makefile
|- src/
|  |- main.rs
|  \- sandbox.rs
\- wasm/
   \- uutils.wasm
```
