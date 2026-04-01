package controlplane
package controlplane

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/groovy-sky/zero-cli/internal/model"
	"github.com/groovy-sky/zero-cli/internal/store"
)

type Server struct {
	store   *store.MemoryStore
	version string
}

func New(st *store.MemoryStore, version string) *Server {
	return &Server{store: st, version: version}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /v1/sandboxes", s.handleListSandboxes)
	mux.HandleFunc("POST /v1/sandboxes", s.handleCreateSandbox)
	mux.HandleFunc("GET /v1/sandboxes/", s.handleGetSandbox)
	mux.HandleFunc("DELETE /v1/sandboxes/", s.handleDeleteSandbox)
	mux.HandleFunc("GET /v1/policies/", s.handleGetPolicy)
	mux.HandleFunc("PUT /v1/policies/", s.handlePutPolicy)
	mux.HandleFunc("GET /v1/providers", s.handleListProviders)
	mux.HandleFunc("POST /v1/providers", s.handleUpsertProvider)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": s.version,
	})
}

func (s *Server) handleCreateSandbox(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name  string `json:"name"`
		Image string `json:"image"`
		Agent string `json:"agent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if request.Name == "" {
		writeError(w, http.StatusBadRequest, errors.New("name is required"))
		return
	}
	sandbox := s.store.CreateSandbox(model.Sandbox{
		Name:  request.Name,
		Image: request.Image,
		Agent: request.Agent,
		State: model.SandboxStatePending,
	})
	writeJSON(w, http.StatusCreated, sandbox)
}

func (s *Server) handleListSandboxes(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"sandboxes": s.store.ListSandboxes()})
}

func (s *Server) handleGetSandbox(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/v1/sandboxes/")
	if name == "" || strings.Contains(name, "/") {
		writeError(w, http.StatusNotFound, store.ErrNotFound)
		return
	}
	sandbox, ok := s.store.GetSandbox(name)
	if !ok {
		writeError(w, http.StatusNotFound, store.ErrNotFound)
		return
	}
	writeJSON(w, http.StatusOK, sandbox)
}

func (s *Server) handleDeleteSandbox(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/v1/sandboxes/")
	if name == "" || strings.Contains(name, "/") {
		writeError(w, http.StatusNotFound, store.ErrNotFound)
		return
	}
	deleted := s.store.DeleteSandbox(name)
	if !deleted {
		writeError(w, http.StatusNotFound, store.ErrNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) handleGetPolicy(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/v1/policies/")
	if name == "" || strings.Contains(name, "/") {
		writeError(w, http.StatusNotFound, store.ErrNotFound)
		return
	}
	pol, ok := s.store.GetPolicy(name)
	if !ok {
		writeError(w, http.StatusNotFound, store.ErrNotFound)
		return
	}
	writeJSON(w, http.StatusOK, pol)
}

func (s *Server) handlePutPolicy(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/v1/policies/")
	if name == "" || strings.Contains(name, "/") {
		writeError(w, http.StatusNotFound, store.ErrNotFound)
		return
	}
	var request struct {
		AllowHosts []string `json:"allow_hosts"`
		AllowPaths []string `json:"allow_paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	pol := s.store.UpsertPolicy(model.Policy{
		SandboxName: name,
		AllowHosts:  request.AllowHosts,
		AllowPaths:  request.AllowPaths,
	})
	writeJSON(w, http.StatusOK, pol)
}

func (s *Server) handleUpsertProvider(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name      string `json:"name"`
		Kind      string `json:"kind"`
		SecretRef string `json:"secret_ref"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if request.Name == "" {
		writeError(w, http.StatusBadRequest, errors.New("name is required"))
		return
	}
	provider := s.store.UpsertProvider(model.Provider{
		Name:      request.Name,
		Kind:      request.Kind,
		SecretRef: request.SecretRef,
	})
	writeJSON(w, http.StatusCreated, provider)
}

func (s *Server) handleListProviders(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"providers": s.store.ListProviders()})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{
		"error":   fmt.Sprintf("%d %s", status, http.StatusText(status)),
		"message": err.Error(),
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}
