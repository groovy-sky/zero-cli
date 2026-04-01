package host
package host

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	"github.com/groovy-sky/zero-cli/internal/model"
	"github.com/groovy-sky/zero-cli/internal/policy"
	"github.com/groovy-sky/zero-cli/internal/runtime"
)

var ErrInvalidWASM = errors.New("invalid wasm module")

type Manager struct {
	modulePath string
}

func NewManager(modulePath string) *Manager {
	return &Manager{modulePath: modulePath}
}

// ValidateModule checks that the file is a valid WASM binary and returns its SHA-256 hash.
func (m *Manager) ValidateModule() (string, error) {
	data, err := os.ReadFile(m.modulePath)
	if err != nil {
		return "", err
	}
	if len(data) < 4 || !bytes.Equal(data[:4], []byte{0x00, 0x61, 0x73, 0x6d}) {
		return "", ErrInvalidWASM
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// Launch validates the WASM module and runs it inside the sandbox defined by pol.
func (m *Manager) Launch(ctx context.Context, sandbox model.Sandbox, pol policy.Policy) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	if sandbox.Name == "" {
		return "", errors.New("sandbox name is required")
	}
	hash, err := m.ValidateModule()
	if err != nil {
		return "", err
	}

	cfg := runtime.RunConfig{
		WasmPath: m.modulePath,
		Policy:   pol,
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
		Stdin:    os.Stdin,
	}
	if err := runtime.RunModule(ctx, cfg); err != nil {
		return "", err
	}
	return fmt.Sprintf("sandbox=%s wasm=%s", sandbox.Name, hash), nil
}
