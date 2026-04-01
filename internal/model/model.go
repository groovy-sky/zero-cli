package model
package model

import "time"

type SandboxState string

const (
	SandboxStatePending SandboxState = "pending"
	SandboxStateRunning SandboxState = "running"
	SandboxStateStopped SandboxState = "stopped"
)

type Sandbox struct {
	Name      string       `json:"name"`
	Image     string       `json:"image,omitempty"`
	Agent     string       `json:"agent,omitempty"`
	State     SandboxState `json:"state"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

type Policy struct {
	SandboxName string    `json:"sandbox_name"`
	AllowHosts  []string  `json:"allow_hosts,omitempty"`
	AllowPaths  []string  `json:"allow_paths,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Provider struct {
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	SecretRef string    `json:"secret_ref,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Session struct {
	Token     string    `json:"token"`
	Sandbox   string    `json:"sandbox"`
	ExpiresAt time.Time `json:"expires_at"`
}
