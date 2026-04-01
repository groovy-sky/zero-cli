# TinyGo Integration Guide

This project is designed to be TinyGo-friendly, with special considerations for building WebAssembly modules.

## Architecture

The project has two distinct parts:

1. **Host CLI** (`cmd/zero`, `internal/*`) - Runs on the host system using standard Go
2. **Guest Modules** (`examples/agent`) - Compiled with TinyGo to WebAssembly (WASI target)

## Building Guest Modules with TinyGo

### Prerequisites

**Important**: TinyGo currently supports Go versions 1.19-1.25. Make sure your Go version is compatible:
```bash
go version  # Should show go1.25 or earlier (not go1.26+)
```

Install TinyGo from https://tinygo.org/getting-started/install/

### Building the Example Agent

```bash
cd examples/agent
tinygo build -o agent.wasm -target=wasi .
```

### Key TinyGo Patterns Used

1. **WebAssembly Imports** - Using `//go:wasmimport` directive:
   ```go
   //go:wasmimport host http_fetch
   func httpFetch(ptr uint32, l uint32) (respPtr uint32, errno uint32)
   ```

2. **Memory Management** - Exporting an `alloc` function for host-to-guest memory allocation:
   ```go
   //export alloc
   func alloc(n uint32) uint32 {
       b := make([]byte, n)
       return uint32(uintptr(unsafe.Pointer(&b[0])))
   }
   ```

3. **Build Constraints** - Using build tags to prevent regular Go from compiling WASM-specific code:
   ```go
   //go:build tinygo.wasm || wasm
   ```

## Design Decisions for TinyGo Compatibility   

### JSON Instead of YAML

The project uses JSON for configuration instead of YAML because:
- JSON is part of Go's standard library (`encoding/json`)
- YAML requires external dependencies that may not work well with TinyGo
- JSON parsing is simpler and more reliable across different Go implementations

### Context-Based Timeouts

The host CLI uses `context.WithTimeout` instead of goroutines with `select` and `time.After`:
```go
ctx := context.Background()
if timeout > 0 {
    var cancel context.CancelFunc
    ctx, cancel = context.WithTimeout(ctx, timeout)
    defer cancel()
}
```

This is more idiomatic and works better with wazero's runtime.

### Minimal Dependencies

The project minimizes dependencies to ensure maximum compatibility:
- **Host dependencies**: Only `wazero` (required for WASM runtime)
- **Guest dependencies**: None! Pure standard library

## Host vs Guest Limitations

### Host CLI (Standard Go)
- Can use full Go standard library
- Can use external dependencies like wazero
- Runs on native OS (Linux, macOS, Windows)
- **Cannot** be compiled with TinyGo (wazero requires full Go)

### Guest Modules (TinyGo)
- Limited to TinyGo-supported standard library subset
- Should avoid complex packages (net/http client, etc.)
- Runs inside WASM sandbox via WASI
- Must use `//go:wasmimport` for host function calls
- **Must** be compiled with TinyGo for small binaries and WASI support

## Security Model

The TinyGo WASM modules run in a secure sandbox:

1. **Filesystem**: Only sees a single preopened directory (`/sandbox`)
2. **Network**: Cannot make direct network calls
3. **Host Functions**: Can call whitelisted host functions like `http_fetch`
4. **Memory**: Isolated linear memory, can only access allocated regions

## Communication Protocol

Guest modules communicate with the host using a simple binary protocol:

### Request (http_fetch)
```
[4 bytes: URL length][URL bytes]
```

### Response
```
[4 bytes: HTTP status][4 bytes: body length][body bytes]
```

## Example: Writing a TinyGo WASM Module

```go
//go:build tinygo.wasm || wasm

package main

import (
    "encoding/binary"
    "fmt"
    "unsafe"
)

//go:wasmimport host http_fetch
func httpFetch(ptr uint32, l uint32) (respPtr uint32, errno uint32)

//go:wasmimport host http_fetch_len
func httpFetchLen() uint32

//export alloc
func alloc(n uint32) uint32 {
    b := make([]byte, n)
    return uint32(uintptr(unsafe.Pointer(&b[0])))
}

func main() {
    // Your WASM code here
    url := "https://api.github.com/zen"
    // ... make request using httpFetch ...
}
```

Build with:
```bash
tinygo build -o mymodule.wasm -target=wasi .
```

Run with:
```bash
./zero run --wasm mymodule.wasm --workspace ./data --policy policy.json
```

## Testing

Host tests run with standard `go test`:
```bash
go test ./...
```

Guest modules should be tested manually or with integration tests that run the full WASM binary.

## Troubleshooting

### "missing function body" Error
This means you're trying to compile WASM code with regular Go. Make sure your WASM code has build constraints:
```go
//go:build tinygo.wasm || wasm
```

### Memory Access Violations
Use the exported `alloc` function to allocate guest memory that the host can write to. Never use arbitrary pointers.

### Import Errors
Ensure host functions are registered in the host runtime before instantiating the guest module.

## Further Reading

- [TinyGo Documentation](https://tinygo.org/)
- [WebAssembly System Interface (WASI)](https://wasi.dev/)
- [wazero Documentation](https://wazero.io/)
