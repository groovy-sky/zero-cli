package zero

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/groovy-sky/zero-cli/internal/controlplane"
	"github.com/groovy-sky/zero-cli/internal/host"
	"github.com/groovy-sky/zero-cli/internal/model"
	"github.com/groovy-sky/zero-cli/internal/policy"
	"github.com/groovy-sky/zero-cli/internal/runtime"
	"github.com/groovy-sky/zero-cli/internal/store"
	"github.com/groovy-sky/zero-cli/internal/version"
)

const defaultServerURL = "http://127.0.0.1:8080"

func Main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "run":
		err = runCmd(args)
	case "serve":
		err = serveCmd(args)
	case "sandbox":
		err = sandboxCmd(args)
	case "policy":
		err = policyCmd(args)
	case "provider":
		err = providerCmd(args)
	case "host":
		err = hostCmd(args)
	case "version":
		fmt.Println(version.Version)
		return
	case "help", "-h", "--help":
		usage()
		return
	default:
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err.Error())
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "zero - TinyGo-friendly WASI/WASM sandbox runner (wazero)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  zero <command> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  run       Run a WASM module in a restricted WASI sandbox")
	fmt.Fprintln(os.Stderr, "  serve     Start the control plane HTTP server")
	fmt.Fprintln(os.Stderr, "  sandbox   Manage sandboxes (create, list, delete, status)")
	fmt.Fprintln(os.Stderr, "  policy    Manage sandbox policies (set, get)")
	fmt.Fprintln(os.Stderr, "  provider  Manage providers (create, list)")
	fmt.Fprintln(os.Stderr, "  host      Manage WASM modules locally (validate, launch)")
	fmt.Fprintln(os.Stderr, "  version   Print version")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Policy files should be in JSON format.")
}

func runCmd(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var wasmPath string
	var workspace string
	var policyPath string

	fs.StringVar(&wasmPath, "wasm", "", "Path to WASM module (.wasm)")
	fs.StringVar(&workspace, "workspace", "", "Host workspace directory to mount into guest")
	fs.StringVar(&policyPath, "policy", "", "Policy JSON file")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if wasmPath == "" || workspace == "" || policyPath == "" {
		fs.Usage()
		return fmt.Errorf("missing required flags")
	}

	var err error

	wasmPath, err = filepath.Abs(wasmPath)
	if err != nil {
		return err
	}

	workspace, err = filepath.Abs(workspace)
	if err != nil {
		return err
	}

	policyPath, err = filepath.Abs(policyPath)
	if err != nil {
		return err
	}

	pol, err := policy.LoadFile(policyPath)
	if err != nil {
		return err
	}

	pol.FS.WorkspaceHost = workspace

	// timeout handling (TinyGo-safe)
	var timeout time.Duration
	if pol.Limits.TimeoutMS > 0 {
		timeout = time.Duration(pol.Limits.TimeoutMS) * time.Millisecond
	}

	cfg := runtime.RunConfig{
		WasmPath: wasmPath,
		Policy:   pol,
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
		Stdin:    os.Stdin,
	}

	st, err := os.Stat(cfg.Policy.FS.WorkspaceHost)
	if err != nil || !st.IsDir() {
		return fmt.Errorf("workspace_host is not a directory: %s", cfg.Policy.FS.WorkspaceHost)
	}

	st, err = os.Stat(cfg.WasmPath)
	if err != nil || st.IsDir() {
		return fmt.Errorf("wasm path is not a file: %s", cfg.WasmPath)
	}

	if cfg.Policy.FS.WorkspaceGuest == "" {
		cfg.Policy.FS.WorkspaceGuest = "/sandbox"
	}

	// run module with timeout context
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	if err := runtime.RunModule(ctx, cfg); err != nil {
		return err
	}

	fmt.Fprintln(
		os.Stderr,
		"ok: ran",
		filepath.Base(wasmPath),
		"with workspace",
		filepath.Base(workspace),
		"mounted at",
		cfg.Policy.FS.WorkspaceGuest,
	)

	return nil
}

// ── serve ─────────────────────────────────────────────────────────────────────

func serveCmd(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:8080", "listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	server := controlplane.New(store.NewMemoryStore(), version.Version)
	fmt.Printf("zero control plane listening on http://%s\n", *addr)
	return http.ListenAndServe(*addr, server.Handler())
}

// ── sandbox ───────────────────────────────────────────────────────────────────

func sandboxCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("sandbox requires a subcommand: create, list, delete, status")
	}
	switch args[0] {
	case "create":
		return sandboxCreate(args[1:])
	case "list":
		return sandboxList(args[1:])
	case "delete":
		return sandboxDelete(args[1:])
	case "status":
		return sandboxStatus(args[1:])
	default:
		return fmt.Errorf("unknown sandbox subcommand: %s", args[0])
	}
}

func sandboxCreate(args []string) error {
	fs := flag.NewFlagSet("sandbox create", flag.ContinueOnError)
	serverURL := fs.String("server", defaultServerURL, "control plane URL")
	name := fs.String("name", "", "sandbox name")
	image := fs.String("image", "ghcr.io/zeroshell/sandbox:latest", "sandbox image")
	agent := fs.String("agent", "tinygo-wasm-agent", "agent name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return errors.New("--name is required")
	}
	body, _ := json.Marshal(map[string]string{
		"name":  *name,
		"image": *image,
		"agent": *agent,
	})
	resp, err := http.Post(*serverURL+"/v1/sandboxes", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return printResponse(resp)
}

func sandboxList(args []string) error {
	fs := flag.NewFlagSet("sandbox list", flag.ContinueOnError)
	serverURL := fs.String("server", defaultServerURL, "control plane URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	resp, err := http.Get(*serverURL + "/v1/sandboxes")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return printResponse(resp)
}

func sandboxDelete(args []string) error {
	fs := flag.NewFlagSet("sandbox delete", flag.ContinueOnError)
	serverURL := fs.String("server", defaultServerURL, "control plane URL")
	name := fs.String("name", "", "sandbox name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return errors.New("--name is required")
	}
	req, err := http.NewRequest(http.MethodDelete, *serverURL+"/v1/sandboxes/"+*name, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return printResponse(resp)
}

func sandboxStatus(args []string) error {
	fs := flag.NewFlagSet("sandbox status", flag.ContinueOnError)
	serverURL := fs.String("server", defaultServerURL, "control plane URL")
	name := fs.String("name", "", "sandbox name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return errors.New("--name is required")
	}
	resp, err := http.Get(*serverURL + "/v1/sandboxes/" + *name)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return printResponse(resp)
}

// ── policy ────────────────────────────────────────────────────────────────────

func policyCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("policy requires a subcommand: set, get")
	}
	switch args[0] {
	case "set":
		return policySet(args[1:])
	case "get":
		return policyGet(args[1:])
	default:
		return fmt.Errorf("unknown policy subcommand: %s", args[0])
	}
}

func policySet(args []string) error {
	fs := flag.NewFlagSet("policy set", flag.ContinueOnError)
	serverURL := fs.String("server", defaultServerURL, "control plane URL")
	sandbox := fs.String("sandbox", "", "sandbox name")
	allowHosts := fs.String("allow-hosts", "", "comma-separated host allow list")
	allowPaths := fs.String("allow-paths", "", "comma-separated path allow list")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *sandbox == "" {
		return errors.New("--sandbox is required")
	}
	body, _ := json.Marshal(map[string][]string{
		"allow_hosts": splitCSV(*allowHosts),
		"allow_paths": splitCSV(*allowPaths),
	})
	req, err := http.NewRequest(http.MethodPut, *serverURL+"/v1/policies/"+*sandbox, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return printResponse(resp)
}

func policyGet(args []string) error {
	fs := flag.NewFlagSet("policy get", flag.ContinueOnError)
	serverURL := fs.String("server", defaultServerURL, "control plane URL")
	sandbox := fs.String("sandbox", "", "sandbox name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *sandbox == "" {
		return errors.New("--sandbox is required")
	}
	resp, err := http.Get(*serverURL + "/v1/policies/" + *sandbox)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return printResponse(resp)
}

// ── provider ──────────────────────────────────────────────────────────────────

func providerCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("provider requires a subcommand: create, list")
	}
	switch args[0] {
	case "create":
		return providerCreate(args[1:])
	case "list":
		return providerList(args[1:])
	default:
		return fmt.Errorf("unknown provider subcommand: %s", args[0])
	}
}

func providerCreate(args []string) error {
	fs := flag.NewFlagSet("provider create", flag.ContinueOnError)
	serverURL := fs.String("server", defaultServerURL, "control plane URL")
	name := fs.String("name", "", "provider name")
	kind := fs.String("kind", "openai", "provider kind")
	secretRef := fs.String("secret-ref", "", "secret reference")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return errors.New("--name is required")
	}
	body, _ := json.Marshal(map[string]string{
		"name":       *name,
		"kind":       *kind,
		"secret_ref": *secretRef,
	})
	resp, err := http.Post(*serverURL+"/v1/providers", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return printResponse(resp)
}

func providerList(args []string) error {
	fs := flag.NewFlagSet("provider list", flag.ContinueOnError)
	serverURL := fs.String("server", defaultServerURL, "control plane URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	resp, err := http.Get(*serverURL + "/v1/providers")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return printResponse(resp)
}

// ── host ──────────────────────────────────────────────────────────────────────

func hostCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("host requires a subcommand: validate, launch")
	}
	switch args[0] {
	case "validate":
		return hostValidate(args[1:])
	case "launch":
		return hostLaunch(args[1:])
	default:
		return fmt.Errorf("unknown host subcommand: %s", args[0])
	}
}

func hostValidate(args []string) error {
	fs := flag.NewFlagSet("host validate", flag.ContinueOnError)
	module := fs.String("module", "", "path to WASM module")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *module == "" {
		return errors.New("--module is required")
	}
	mgr := host.NewManager(*module)
	hash, err := mgr.ValidateModule()
	if err != nil {
		return err
	}
	fmt.Println(hash)
	return nil
}

func hostLaunch(args []string) error {
	fs := flag.NewFlagSet("host launch", flag.ContinueOnError)
	module := fs.String("module", "", "path to WASM module")
	sandboxName := fs.String("sandbox", "", "sandbox name")
	policyPath := fs.String("policy", "", "policy JSON file")
	workspace := fs.String("workspace", "", "host workspace directory to mount")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *module == "" {
		return errors.New("--module is required")
	}
	if *sandboxName == "" {
		return errors.New("--sandbox is required")
	}
	if *policyPath == "" {
		return errors.New("--policy is required")
	}
	if *workspace == "" {
		return errors.New("--workspace is required")
	}

	pol, err := policy.LoadFile(*policyPath)
	if err != nil {
		return err
	}
	pol.FS.WorkspaceHost = *workspace

	mgr := host.NewManager(*module)
	result, err := mgr.Launch(context.Background(), model.Sandbox{Name: *sandboxName}, pol)
	if err != nil {
		return err
	}
	fmt.Println(result)
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func printResponse(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		fmt.Println(resp.Status)
		return nil
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	fmt.Println(string(body))
	return nil
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func init() {
	http.DefaultClient.Timeout = 10 * time.Second
}
