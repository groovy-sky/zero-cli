package runtime

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/groovy-sky/zero-cli/internal/hostapi"
	"github.com/groovy-sky/zero-cli/internal/policy"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

type RunConfig struct {
	WasmPath string
	Policy   policy.Policy

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

func RunModule(ctx context.Context, cfg RunConfig) error {
	// Create runtime config with memory limits if specified
	rtConfig := wazero.NewRuntimeConfig()
	if cfg.Policy.Limits.MaxMemoryMB > 0 {
		// Convert MB to pages (1 page = 64KB)
		limit := uint64(cfg.Policy.Limits.MaxMemoryMB) << 20
		pages := uint32(limit / 65536)
		rtConfig = rtConfig.WithMemoryLimitPages(pages)
	}
	
	rt := wazero.NewRuntimeWithConfig(ctx, rtConfig)
	defer rt.Close(ctx)

	// Instantiate WASI
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		return fmt.Errorf("instantiate WASI: %w", err)
	}

	// Host module with http_fetch (optional depending on policy.net.enabled)
	if cfg.Policy.Net.Enabled {
		h := hostapi.NewHTTPFetcher(cfg.Policy)
		builder := rt.NewHostModuleBuilder("host")
		builder = h.Export("host", nil, builder)
		if _, err := builder.Instantiate(ctx); err != nil {
			return fmt.Errorf("instantiate host module: %w", err)
		}
	}

	// Configure filesystem and stdio
	hostDir := cfg.Policy.FS.WorkspaceHost
	guestDir := cfg.Policy.FS.WorkspaceGuest
	if hostDir == "" {
		hostDir = "."
	}
	if guestDir == "" {
		guestDir = "/sandbox"
	}

	fsc := wazero.NewFSConfig().WithDirMount(hostDir, guestDir)
	wc := wazero.NewModuleConfig().
		WithStdout(cfg.Stdout).
		WithStderr(cfg.Stderr).
		WithStdin(cfg.Stdin).
		WithArgs(cfg.WasmPath).
		WithFSConfig(fsc)

	wasmBytes, err := os.ReadFile(cfg.WasmPath)
	if err != nil {
		return err
	}

	compiled, err := rt.CompileModule(ctx, wasmBytes)
	if err != nil {
		return fmt.Errorf("compile module: %w", err)
	}
	defer compiled.Close(ctx)

	mod, err := rt.InstantiateModule(ctx, compiled, wc)
	if err != nil {
		return fmt.Errorf("instantiate module: %w", err)
	}
	defer mod.Close(ctx)

	// Call _start if it exists (WASI convention)
	start := mod.ExportedFunction("_start")
	if start != nil {
		if _, err := start.Call(ctx); err != nil {
			return fmt.Errorf("_start failed: %w", err)
		}
	}
	return nil
}
