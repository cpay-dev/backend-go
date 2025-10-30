package authn_test

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cpay-dev/backend-go/internal/authn"
	authnpb "github.com/cpay-dev/proto-go/api/v1/authn"
)

func TestAuthURL_GoogleConfigured(t *testing.T) {
	store := authn.NewMemoryInitStateStore()
	svc := authn.NewOAuthService(authn.ProvidersConfig{Google: authn.GoogleProvider{
		ClientID:    "client-id",
		RedirectURI: "http://localhost/callback",
		Scopes:      []string{"openid", "email"},
	}}, store)

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	t.Cleanup(cancel)
	u, err := svc.AuthURL(ctx, authnpb.AuthProvider_AUTH_PROVIDER_GOOGLE, "nonce", "/redirect")
	require.NoError(t, err, "AuthURL returned error for Google: %v", err)
	require.NotEmpty(t, u, "AuthURL returned empty URL for Google provider")

	parsed, err := url.Parse(u)
	require.NoError(t, err, "failed parsing URL: %s (err=%v)", u, err)
	q := parsed.Query()
	require.Equal(t, "client-id", q.Get("client_id"), "unexpected client_id in URL: %s", u)
	require.Equal(t, "http://localhost/callback", q.Get("redirect_uri"), "unexpected redirect_uri in URL: %s", u)
	// Access type param
	require.Equal(t, "offline", q.Get("access_type"), "missing or incorrect access_type in URL: %s", u)
	// Scopes are space-delimited; ensure at least one we set is present
	require.Contains(t, q.Get("scope"), "openid", "expected 'openid' scope in URL: %s", u)

	// Validate init state persisted in the store
	state := q.Get("state")
	require.NotEmpty(t, state, "missing state in URL: %s", u)
	st, err := store.Pop(ctx, "authn:init:"+state)
	require.NoError(t, err, "expected init state in store for key %q", "authn:init:"+state)
	require.Equal(t, state, st.ID)
	require.Equal(t, authnpb.AuthProvider_AUTH_PROVIDER_GOOGLE, st.Provider)
	require.Equal(t, "nonce", st.Nonce)
	require.Equal(t, "/redirect", st.RedirectPath)
	require.WithinDuration(t, time.Now().UTC(), st.CreatedAt, 2*time.Second)
}

func TestAuthURL_UnsupportedProvider(t *testing.T) {
	svc := authn.NewOAuthService(authn.ProvidersConfig{}, authn.NewMemoryInitStateStore())
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	t.Cleanup(cancel)
	_, err := svc.AuthURL(ctx, authnpb.AuthProvider_AUTH_PROVIDER_UNSPECIFIED, "n", "/r")
	require.ErrorIs(t, err, authn.ErrProviderUnsupported, "expected ErrProviderUnsupported; got: %v", err)
}
