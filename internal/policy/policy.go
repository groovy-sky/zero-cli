package policy

import (
	"encoding/json"
	"fmt"
	"os"
)

type Policy struct {
	FS     FS     `json:"fs"`
	Net    Net    `json:"net"`
	Limits Limits `json:"limits"`
}

type FS struct {
	WorkspaceHost  string `json:"workspace_host"`
	WorkspaceGuest string `json:"workspace_guest"`
}

type Net struct {
	Enabled   bool     `json:"enabled"`
	HTTPAllow []string `json:"http_allow"`
}

type Limits struct {
	TimeoutMS   int64 `json:"timeout_ms"`
	MaxMemoryMB int64 `json:"max_memory_mb"`
}

func LoadFile(path string) (Policy, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, err
	}
	return LoadBytes(b)
}

func LoadBytes(b []byte) (Policy, error) {
	var p Policy
	if err := json.Unmarshal(b, &p); err != nil {
		return Policy{}, err
	}
	if p.FS.WorkspaceGuest == "" {
		p.FS.WorkspaceGuest = "/sandbox"
	}
	// Normalize: if net is enabled, allowlist must be non-empty
	if p.Net.Enabled && len(p.Net.HTTPAllow) == 0 {
		return Policy{}, fmt.Errorf("net.enabled=true but net.http_allow is empty")
	}
	return p, nil
}

func (p Policy) IsURLAllowed(url string) bool {
	if !p.Net.Enabled {
		return false
	}
	for _, prefix := range p.Net.HTTPAllow {
		if prefix != "" && len(url) >= len(prefix) && url[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}
