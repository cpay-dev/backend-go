package authn

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrStateNotFound is returned when the key does not exist.
	ErrStateNotFound = errors.New("init state not found")
	// ErrStateExpired is returned when the key is present but expired.
	ErrStateExpired = errors.New("init state expired")
)

// InitStateStore persists OAuth initiation state with a TTL and allows one-time retrieval.
type InitStateStore interface {
	Save(ctx context.Context, key string, state OAuthInitState, ttl time.Duration) error
	Pop(ctx context.Context, key string) (OAuthInitState, error)
}
