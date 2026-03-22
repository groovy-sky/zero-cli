# zero-cli

`zero` is a lightweight WASI/WebAssembly sandbox runner built on top of [wazero].

It is designed for **Option A**: run a WASM module with a minimal capability set:

- Filesystem access only to a single pre-opened workspace directory mounted at `/sandbox`.
- Network access only via an explicit host function `http_fetch`, gated by a URL allowlist in policy.

## Requirements

- Go (host CLI). This repo targets Go 1.17+.
- TinyGo (to compile the example agent to WASI). Tested with the TinyGo version declared in `go.mod`.

## Quick start

### 1) Build the CLI

```bash
go build ./cmd/zero
