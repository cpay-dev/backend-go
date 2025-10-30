//go:build docker
// +build docker

package authn_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	valkeytestcontainers "github.com/testcontainers/testcontainers-go/modules/valkey"
	"github.com/valkey-io/valkey-go"

	"github.com/cpay-dev/backend-go/internal/authn"
)

func newValkeyClient(t *testing.T) valkey.Client {
	t.Helper()

	container, err := valkeytestcontainers.Run(t.Context(), "valkey/valkey:9.0-alpine")
	testcontainers.CleanupContainer(t, container)

	host, err := container.Host(t.Context())
	require.NoError(t, err, "get container host")
	port, err := container.MappedPort(t.Context(), "6379/tcp")
	require.NoError(t, err, "map container port")

	addr := fmt.Sprintf("%s:%s", host, port.Port())
	cli, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{addr}})
	require.NoError(t, err, "valkey: new client")
	t.Cleanup(cli.Close)
	return cli
}

func TestValkeyInitStateStore_SaveAndPop(t *testing.T) {
	t.Parallel()

	cli := newValkeyClient(t)
	store := authn.NewValkeyInitStateStore(authn.NewValkeyKVAdapter(cli))

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)

	key := "authn:init:test-save-pop"
	expected := authn.OAuthInitState{
		ID:           "state-1",
		Provider:     1, // authnpb.AuthProvider_AUTH_PROVIDER_GOOGLE
		Nonce:        "nonce-xyz",
		RedirectPath: "/welcome",
		CreatedAt:    time.Now().UTC(),
	}

	require.NoError(t, store.Save(ctx, key, expected, 15*time.Second), "save state")

	// First Pop should return the saved state
	got, err := store.Pop(ctx, key)
	require.NoError(t, err, "pop state")
	require.Equal(t, expected.ID, got.ID)
	require.Equal(t, expected.Provider, got.Provider)
	require.Equal(t, expected.Nonce, got.Nonce)
	require.Equal(t, expected.RedirectPath, got.RedirectPath)
	require.WithinDuration(t, expected.CreatedAt, got.CreatedAt, time.Second)

	// Second Pop should return not found
	_, err = store.Pop(ctx, key)
	require.ErrorIs(t, err, authn.ErrStateNotFound, "second pop should not find key")
}

func TestValkeyInitStateStore_Expiry(t *testing.T) {
	t.Parallel()

	cli := newValkeyClient(t)
	store := authn.NewValkeyInitStateStore(authn.NewValkeyKVAdapter(cli))

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	t.Cleanup(cancel)

	key := "authn:init:test-expiry"
	st := authn.OAuthInitState{ID: "exp-1", CreatedAt: time.Now().UTC()}
	require.NoError(t, store.Save(ctx, key, st, 1*time.Second), "save state with short TTL")

	time.Sleep(1500 * time.Millisecond)

	_, err := store.Pop(ctx, key)
	// For Valkey-backed store, expired keys are gone -> ErrStateNotFound
	require.ErrorIs(t, err, authn.ErrStateNotFound, "expired state should not be found")
}
