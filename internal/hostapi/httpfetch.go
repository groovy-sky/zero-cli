package hostapi

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/groovy-sky/zero-cli/internal/policy"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// http_fetch ABI
//
// Guest calls:
//   http_fetch(ptr, len) -> u32 (pointer to response bytes in guest memory)
//   http_fetch_len() -> u32 (length of response bytes)
//
// Request bytes format (little endian):
//   u32 urlLen | urlBytes
//
// Response bytes format (little endian):
//   u32 status | u32 bodyLen | bodyBytes
//
// This is intentionally minimal. You can extend later to include method, headers, etc.

type HTTPFetcher struct {
	Policy policy.Policy

	client *http.Client

	// response scratch, returned via http_fetch_len()
	lastResp []byte
}

func NewHTTPFetcher(pol policy.Policy) *HTTPFetcher {
	// Keep it simple; set a short-ish timeout. Outer context deadline also applies.
	return &HTTPFetcher{
		Policy: pol,
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

// Exported functions:
// - "http_fetch": (i32 ptr, i32 len) -> i32 respPtr
// - "http_fetch_len": () -> i32 respLen
func (h *HTTPFetcher) Export(moduleName string, r api.Module, b wazero.HostModuleBuilder) wazero.HostModuleBuilder {
	_ = moduleName
	_ = r

	b = b.NewFunctionBuilder().
		WithName("http_fetch_len").
		WithFunc(func(ctx context.Context, m api.Module) uint32 {
			return uint32(len(h.lastResp))
		}).
		Export("http_fetch_len")

	b = b.NewFunctionBuilder().
		WithName("http_fetch").
		WithFunc(func(ctx context.Context, m api.Module, ptr uint32, l uint32) (uint32, uint32) {
			// Returns (respPtr, errno)
			// errno=0 ok, errno=1 bad request, errno=2 denied, errno=3 internal
			reqBytes, ok := readBytes(m, ptr, l)
			if !ok {
				h.lastResp = nil
				return 0, 1
			}

			url, err := decodeURL(reqBytes)
			if err != nil {
				h.lastResp = nil
				return 0, 1
			}

			if !h.Policy.IsURLAllowed(url) {
				h.lastResp = encodeResponse(403, []byte("policy_denied"))
				respPtr, ok := writeToGuest(m, h.lastResp)
				if !ok {
					return 0, 3
				}
				return respPtr, 2
			}

			body, status, err := h.doFetch(ctx, url)
			if err != nil {
				h.lastResp = encodeResponse(502, []byte(err.Error()))
			} else {
				h.lastResp = encodeResponse(status, body)
			}

			respPtr, ok := writeToGuest(m, h.lastResp)
			if !ok {
				return 0, 3
			}
			return respPtr, 0
		}).
		Export("http_fetch")

	return b
}

func (h *HTTPFetcher) doFetch(ctx context.Context, url string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	// Limit response size to keep memory bounded.
	const max = 1 << 20 // 1 MiB
	b, err := io.ReadAll(io.LimitReader(resp.Body, max))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return b, resp.StatusCode, nil
}

func decodeURL(b []byte) (string, error) {
	if len(b) < 4 {
		return "", fmt.Errorf("bad request")
	}
	urlLen := binary.LittleEndian.Uint32(b[:4])
	if int(urlLen) > len(b)-4 {
		return "", fmt.Errorf("bad request")
	}
	return string(b[4 : 4+urlLen]), nil
}

func encodeResponse(status int, body []byte) []byte {
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, uint32(status))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(len(body)))
	_, _ = buf.Write(body)
	return buf.Bytes()
}

func readBytes(m api.Module, ptr, l uint32) ([]byte, bool) {
	mem := m.Memory()
	b, ok := mem.Read(ptr, l)
	if !ok {
		return nil, false
	}
	// Copy because underlying memory is mutable
	out := make([]byte, len(b))
	copy(out, b)
	return out, true
}

// writeToGuest allocates a guest buffer by calling malloc-like export if present,
// otherwise writes to a fixed scratch region (not great) -> we require "alloc".
func writeToGuest(m api.Module, b []byte) (uint32, bool) {
	alloc := m.ExportedFunction("alloc")
	if alloc == nil {
		return 0, false
	}
	res, err := alloc.Call(context.Background(), uint64(len(b)))
	if err != nil || len(res) == 0 {
		return 0, false
	}
	ptr := uint32(res[0])

	mem := m.Memory()
	if !mem.Write(ptr, b) {
		return 0, false
	}
	return ptr, true
}
