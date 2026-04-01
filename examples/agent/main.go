//go:build tinygo.wasm || wasm

// TinyGo WASI example agent for zero-cli.
//
// Build (TinyGo):
//   tinygo build -o agent.wasm -target=wasi .
//
// This module expects the host to provide:
//   - WASI with a preopened directory at /sandbox
//   - host functions in module "host":
//       http_fetch(ptr,len) -> (respPtr, errno)
//       http_fetch_len() -> respLen
//
// It also exports:
//   alloc(n) -> ptr
//
// so the host can allocate response buffers in guest memory.

package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"unsafe"
)

//go:wasmimport host http_fetch
func httpFetch(ptr uint32, l uint32) (respPtr uint32, errno uint32)

//go:wasmimport host http_fetch_len
func httpFetchLen() uint32

//export alloc
func alloc(n uint32) uint32 {
	// TinyGo allocator is fine; make a byte slice and return pointer.
	b := make([]byte, n)
	return uint32(uintptr(unsafe.Pointer(&b[0])))
}

func main() {
	// Write to sandbox
	_ = os.WriteFile("/sandbox/hello.txt", []byte("hello from wasm\n"), 0644)

	// Make allowlisted request
	url := "https://api.github.com/zen"
	req := encodeURL(url)

	reqPtr := alloc(uint32(len(req)))
	memWrite(reqPtr, req)

	respPtr, errno := httpFetch(reqPtr, uint32(len(req)))
	if errno != 0 {
		fmt.Printf("http_fetch denied/failed errno=%d\n", errno)
		return
	}

	respLen := httpFetchLen()
	respBytes := memRead(respPtr, respLen)

	status, body := decodeResponse(respBytes)
	fmt.Printf("status=%d body=%s\n", status, string(body))
}

func encodeURL(url string) []byte {
	// u32 urlLen + url bytes
	out := make([]byte, 4+len(url))
	binary.LittleEndian.PutUint32(out[:4], uint32(len(url)))
	copy(out[4:], []byte(url))
	return out
}

func decodeResponse(b []byte) (uint32, []byte) {
	if len(b) < 8 {
		return 0, nil
	}
	status := binary.LittleEndian.Uint32(b[:4])
	bodyLen := binary.LittleEndian.Uint32(b[4:8])
	if int(bodyLen) > len(b)-8 {
		return status, nil
	}
	return status, b[8 : 8+bodyLen]
}

// TinyGo/WASM linear memory helpers

func memWrite(ptr uint32, b []byte) {
	dst := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), len(b))
	copy(dst, b)
}

func memRead(ptr uint32, n uint32) []byte {
	src := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), int(n))
	out := make([]byte, n)
	copy(out, src)
	return out
}
