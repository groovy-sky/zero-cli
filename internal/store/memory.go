package store
package store

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/groovy-sky/zero-cli/internal/model"
)

var ErrNotFound = errors.New("not found")

type MemoryStore struct {
	mu        sync.RWMutex
	sandboxes map[string]model.Sandbox
	policies  map[string]model.Policy
	providers map[string]model.Provider
	sessions  map[string]model.Session
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		sandboxes: make(map[string]model.Sandbox),
		policies:  make(map[string]model.Policy),
		providers: make(map[string]model.Provider),
		sessions:  make(map[string]model.Session),
	}
}

func (s *MemoryStore) CreateSandbox(sandbox model.Sandbox) model.Sandbox {
	s.mu.Lock()
	defer s.mu.Unlock()
	sandbox.CreatedAt = time.Now().UTC()
	sandbox.UpdatedAt = sandbox.CreatedAt
	if sandbox.State == "" {
		sandbox.State = model.SandboxStatePending
	}
	s.sandboxes[sandbox.Name] = sandbox
	return sandbox
}

func (s *MemoryStore) ListSandboxes() []model.Sandbox {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]model.Sandbox, 0, len(s.sandboxes))
	for _, sandbox := range s.sandboxes {
		items = append(items, sandbox)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func (s *MemoryStore) GetSandbox(name string) (model.Sandbox, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sandbox, ok := s.sandboxes[name]
	return sandbox, ok
}

func (s *MemoryStore) DeleteSandbox(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sandboxes[name]; !ok {
		return false
	}
	delete(s.sandboxes, name)
	delete(s.policies, name)
	return true
}

func (s *MemoryStore) UpsertPolicy(policy model.Policy) model.Policy {
	s.mu.Lock()
	defer s.mu.Unlock()
	policy.UpdatedAt = time.Now().UTC()
	s.policies[policy.SandboxName] = policy
	return policy
}

func (s *MemoryStore) GetPolicy(name string) (model.Policy, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	policy, ok := s.policies[name]
	return policy, ok
}

func (s *MemoryStore) UpsertProvider(provider model.Provider) model.Provider {
	s.mu.Lock()
	defer s.mu.Unlock()
	provider.CreatedAt = time.Now().UTC()
	s.providers[provider.Name] = provider
	return provider
}

func (s *MemoryStore) ListProviders() []model.Provider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]model.Provider, 0, len(s.providers))
	for _, provider := range s.providers {
		items = append(items, provider)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func (s *MemoryStore) GetProvider(name string) (model.Provider, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	provider, ok := s.providers[name]
	return provider, ok
}

func (s *MemoryStore) UpsertSession(session model.Session) model.Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.Token] = session
	return session
}

func (s *MemoryStore) GetSession(token string) (model.Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[token]
	return session, ok
}
