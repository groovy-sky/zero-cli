use anyhow::{bail, Context, Result, anyhow};
use std::collections::HashSet;
use std::path::PathBuf;
use wasmtime::{Engine, Linker, Module, Store};
use wasmtime_wasi::{
    p1::{self, WasiP1Ctx},
    DirPerms, FilePerms, WasiCtxBuilder,
};

/// Output collected from a single sandbox run.
pub struct RunOutput {
    pub stdout: String,
    pub stderr: String,
    pub exit_code: i32,
}

/// Holds a pre-compiled uutils module ready to instantiate.
pub struct Sandbox {
    engine: Engine,
    module: Module,
    linker: Linker<WasiP1Ctx>,
}

impl Sandbox {
    /// Load and compile `uutils.wasm` once. Call this at startup.
    pub fn new(wasm_bytes: &[u8]) -> Result<Self> {
        let engine = Engine::default();
        let module = Module::new(&engine, wasm_bytes)
            .map_err(|e| anyhow!("failed to compile uutils.wasm: {e:#}"))?;

        let mut linker: Linker<WasiP1Ctx> = Linker::new(&engine);
        p1::add_to_linker_sync(&mut linker, |ctx| ctx)
            .map_err(|e| anyhow!("failed to add WASI p1 to linker: {e:#}"))?;

        Ok(Self { engine, module, linker })
    }

    /// Run a single coreutils applet in a fresh WASI instance.
    ///
    /// * `command`       — applet name, e.g. `"ls"`
    /// * `args`          — arguments forwarded verbatim
    /// * `stdin`         — bytes piped to the process's stdin
    /// * `allowed_paths` — host paths exposed to the sandbox (read-write)
    pub fn run(
        &self,
        command: &str,
        args: &[String],
        stdin: &[u8],
        allowed_paths: &[String],
    ) -> Result<RunOutput> {
        // Validate and canonicalize allowed paths before opening them.
        let canonical_paths = Self::validate_paths(allowed_paths)?;

        // Build WASI context.
        let stdout_pipe = wasmtime_wasi::p2::pipe::MemoryOutputPipe::new(4 * 1024 * 1024);
        let stderr_pipe = wasmtime_wasi::p2::pipe::MemoryOutputPipe::new(1 * 1024 * 1024);
        let stdin_pipe = wasmtime_wasi::p2::pipe::MemoryInputPipe::new(stdin.to_vec());

        let mut builder = WasiCtxBuilder::new();
        builder
            .stdin(stdin_pipe)
            .stdout(stdout_pipe.clone())
            .stderr(stderr_pipe.clone())
            // argv[0] = "coreutils", argv[1] = command name (multi-call convention)
            .arg("coreutils")
            .arg(command);

        let mut effective_args = args.to_vec();
        if command == "ls" {
            // In this sandbox, `ls -la` with no operand may probe `./..` and
            // fail due capability boundaries. Default to `/` for parity with
            // expected directory listing behavior.
            let mut has_operand = false;
            let mut end_of_options = false;
            for arg in args {
                if end_of_options {
                    has_operand = true;
                    break;
                }
                if arg == "--" {
                    end_of_options = true;
                    continue;
                }
                if arg == "-" || !arg.starts_with('-') {
                    has_operand = true;
                    break;
                }
            }
            if !has_operand {
                effective_args.push("/".to_string());
            }
        }

        for a in &effective_args {
            builder.arg(a);
        }

        // Expose each allowed path as a preopened dir. Network is not provided
        // (WasiCtxBuilder::new() leaves TCP/UDP disabled by default).
        //
        // The FIRST allowed path is also mounted at "/", ".", and ".." so
        // relative lookups and parent probes stay inside the same sandbox root.
        for (i, (host_path, guest_path)) in canonical_paths.iter().enumerate() {
            if i == 0 {
                builder
                    .preopened_dir(host_path, "/", DirPerms::all(), FilePerms::all())
                    .map_err(|e| anyhow!("failed to open '{}' as /: {e:#}", host_path.display()))?;
                builder
                    .preopened_dir(host_path, ".", DirPerms::all(), FilePerms::all())
                    .map_err(|e| anyhow!("failed to open '{}' as .: {e:#}", host_path.display()))?;
                builder
                    .preopened_dir(host_path, "..", DirPerms::all(), FilePerms::all())
                    .map_err(|e| anyhow!("failed to open '{}' as ..: {e:#}", host_path.display()))?;
            }
            builder
                .preopened_dir(host_path, guest_path, DirPerms::all(), FilePerms::all())
                .map_err(|e| anyhow!("failed to open '{}': {e:#}", host_path.display()))?;
        }

        let wasi_ctx = builder.build_p1();
        let mut store = Store::new(&self.engine, wasi_ctx);

        // Instantiate and call _start.
        let instance = self
            .linker
            .instantiate(&mut store, &self.module)
            .map_err(|e| anyhow!("failed to instantiate uutils.wasm: {e:#}"))?;

        let start = instance
            .get_typed_func::<(), ()>(&mut store, "_start")
            .map_err(|e| anyhow!("uutils.wasm has no _start export: {e:#}"))?;

        let exit_code = match start.call(&mut store, ()) {
            Ok(()) => 0,
            Err(trap) => {
                // proc_exit is signalled as a trap carrying I32Exit.
                if let Some(exit) = trap.downcast_ref::<wasmtime_wasi::I32Exit>() as Option<&wasmtime_wasi::I32Exit> {
                    exit.0
                } else {
                    // Unexpected wasm trap (stack overflow, OOM, etc.) —
                    // log to stderr and map to exit_code = -1 so the caller
                    // always receives a structured result, never an error.
                    tracing::warn!("wasm trap (not proc_exit): {trap:#}");
                    -1
                }
            }
        };

        let stdout = String::from_utf8_lossy(&stdout_pipe.contents()).into_owned();
        let stderr = String::from_utf8_lossy(&stderr_pipe.contents()).into_owned();

        Ok(RunOutput { stdout, stderr, exit_code })
    }

    /// Canonicalize and return `(host_path, guest_path)` pairs.
    /// Each allowed path is exposed inside the sandbox using its last component
    /// as the guest name (e.g. `/tmp/work` → `/work`).
    fn validate_paths(allowed_paths: &[String]) -> Result<Vec<(PathBuf, String)>> {
        let mut result = Vec::with_capacity(allowed_paths.len());
        let mut seen_hosts: HashSet<PathBuf> = HashSet::new();
        let mut seen_guests: HashSet<String> = HashSet::new();
        for raw in allowed_paths {
            let host = std::fs::canonicalize(raw)
                .with_context(|| format!("cannot canonicalize path '{raw}'"))?;

            if !host.is_dir() {
                bail!("allowed_paths entry '{raw}' is not a directory");
            }

            // Silently deduplicate identical canonical host paths.
            if !seen_hosts.insert(host.clone()) {
                continue;
            }

            let name = host
                .file_name()
                .and_then(|n| n.to_str())
                .unwrap_or("root");

            let guest = format!("/{name}");
            if !seen_guests.insert(guest.clone()) {
                bail!(
                    "allowed_paths collision: '{}' and another path both map to guest path '{guest}'",
                    host.display()
                );
            }
            result.push((host, guest));
        }
        Ok(result)
    }
}
