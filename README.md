# zero-cli

`zero` is a **TinyGo-friendly** lightweight WASI/WebAssembly sandbox runner built on top of `wazero`.

This implements **WASM-only isolation**:

- Filesystem access is limited to a single pre-opened workspace directory mounted at `/sandbox`.
- Network access is **disabled by default** and is only available via an explicit host function `http_fetch`,
  gated by a URL allowlist loaded from a policy JSON file.

## Requirements

- Go 1.25 (host CLI) - **Note**: TinyGo currently supports Go 1.19-1.25, not Go 1.26+
- TinyGo (for building WASM guest modules to WASI target)

## Build

### Host CLI

```bash
go build -o zero ./cmd/zero
```

### Example WASM module (TinyGo)

```bash
tinygo build -o examples/agent/agent.wasm -target=wasi ./examples/agent
```

## Run

```bash
./zero run \
  --wasm examples/agent/agent.wasm \
  --workspace . \
  --policy policy.example.json
```

## Policy

See [policy.example.json](policy.example.json). The policy file uses JSON format (better TinyGo compatibility than YAML).

Example policy:
```json
{
  "fs": {
    "workspace_host": ".",
    "workspace_guest": "/sandbox"
  },
  "net": {
    "enabled": true,
    "http_allow": [
      "https://api.github.com/",
      "https://pypi.org/"
    ]
  },
  "limits": {
    "timeout_ms": 30000,
    "max_memory_mb": 128
  }
}
```

## Security model (minimal)

- The guest module cannot open arbitrary host files. It only sees `/sandbox`.
- The guest module cannot access the network unless you enable it in policy.
- Even when enabled, it can only make HTTP(S) requests through `http_fetch`, and only to allowed URL prefixes.

This is intentionally minimal and lightweight. If you need stronger guarantees (DNS pinning, raw TCP blocking,
TLS MITM inspection, etc.), that requires a more complex design.

## TinyGo Compatibility

This project is designed with TinyGo in mind:

- **Guest modules**: Write your WASM modules in TinyGo for small binaries and fast execution
- **Policy format**: Uses JSON instead of YAML for better TinyGo stdlib support
- **Minimal dependencies**: Only essential packages (wazero for host, standard library otherwise)
- **Simple patterns**: Avoids reflection and complex standard library features where possible

The host CLI uses wazero (which requires full Go), but all guest code can be pure TinyGo.

## Notes

- The URL allowlist is **prefix-based** (simple + fast). Example: allowing `https://api.github.com/` allows
  any URL starting with that prefix.
- This project avoids adding a full CLI framework (cobra) to keep dependencies small and TinyGo-friendly.
- Uses JSON for configuration instead of YAML (better standard library support)

---

## Dependencies

- [wazero](https://github.com/tetratelabs/wazero) - WebAssembly runtime
- [TinyGo](https://github.com/tinygo-org/tinygo) - Go compiler for small places