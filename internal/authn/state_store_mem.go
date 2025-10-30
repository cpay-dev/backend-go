package authn

import (
	"context"
	"sync"
	"time"
)

// MemoryInitStateStore is a simple in-memory implementation of InitStateStore.
// Useful for tests and local runs without Valkey.
type MemoryInitStateStore struct {
	mu sync.Mutex
	m  map[string]OAuthInitState
	ex map[string]time.Time
}

func NewMemoryInitStateStore() *MemoryInitStateStore {
	return &MemoryInitStateStore{m: make(map[string]OAuthInitState), ex: make(map[string]time.Time)}
}

func (s *MemoryInitStateStore) Save(_ context.Context, key string, state OAuthInitState, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = state
	s.ex[key] = time.Now().Add(ttl)
	return nil
}

func (s *MemoryInitStateStore) Pop(_ context.Context, key string) (OAuthInitState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.m[key]
	if !ok {
		return OAuthInitState{}, ErrStateNotFound
	}
	exp := s.ex[key]
	delete(s.m, key)
	delete(s.ex, key)
	if time.Now().After(exp) {
		return OAuthInitState{}, ErrStateExpired
	}
	return st, nil
}
