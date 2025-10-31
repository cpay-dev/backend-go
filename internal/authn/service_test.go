package authn_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cpay-dev/backend-go/internal/authn"
	authnpb "github.com/cpay-dev/proto-go/api/v1/authn"
)

func TestInitProviderAuth_GoogleConfigured(t *testing.T) {
	store := authn.NewMemoryInitStateStore()
	svc := authn.NewOAuthProviderService(authn.ProvidersConfig{Google: authn.GoogleProvider{
		ClientID:    "client-id",
		RedirectURI: "http://localhost/callback",
		Scopes:      []string{"openid", "email"},
	}}, store)

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	t.Cleanup(cancel)
	res, err := svc.InitProviderAuth(ctx, authnpb.AuthProvider_AUTH_PROVIDER_GOOGLE)
	require.NoError(t, err, "InitProviderAuth returned error for Google: %v", err)
	require.NotEmpty(t, res.RedirectURL, "InitProviderAuth returned empty URL for Google provider")

	parsed, err := url.Parse(res.RedirectURL)
	require.NoError(t, err, "failed parsing URL: %s (err=%v)", res.RedirectURL, err)
	q := parsed.Query()
	require.Equal(t, "client-id", q.Get("client_id"), "unexpected client_id in URL: %s", res.RedirectURL)
	require.Equal(t, "http://localhost/callback", q.Get("redirect_uri"), "unexpected redirect_uri in URL: %s", res.RedirectURL)
	scopeVals := strings.Fields(q.Get("scope"))
	scopeSet := map[string]struct{}{}
	for _, s := range scopeVals {
		if s == "" {
			continue
		}
		if _, dup := scopeSet[s]; dup {
			require.Failf(t, "duplicate scope", "scope %q appears more than once in URL: %s", s, res.RedirectURL)
		}
		scopeSet[s] = struct{}{}
	}
	for _, expectedScope := range []string{"openid", "email"} {
		_, ok := scopeSet[expectedScope]
		require.True(t, ok, "expected scope %q in URL: %s", expectedScope, res.RedirectURL)
	}
	// PKCE parameters
	require.NotEmpty(t, q.Get("code_challenge"), "expected code_challenge in URL: %s", res.RedirectURL)
	require.Equal(t, "S256", q.Get("code_challenge_method"), "expected code_challenge_method=S256 in URL: %s", res.RedirectURL)
	// OIDC nonce should be present
	require.NotEmpty(t, q.Get("nonce"), "expected nonce in URL: %s", res.RedirectURL)

	// Validate init state persisted in the store
	state := q.Get("state")
	require.NotEmpty(t, state, "missing state in URL: %s", res.RedirectURL)
	st, err := store.Pop(ctx, "authn:init:"+state)
	require.NoError(t, err, "expected init state in store for key %q", "authn:init:"+state)
	require.Equal(t, state, st.ID)
	require.Equal(t, authnpb.AuthProvider_AUTH_PROVIDER_GOOGLE, st.Provider)
	require.NotEmpty(t, st.Nonce)
	require.Equal(t, q.Get("nonce"), st.Nonce, "nonce in URL must match stored state")
	require.NotEmpty(t, st.PKCEVerifier)
	require.Equal(t, q.Get("code_challenge"), st.PKCEChallenge)

	sum := sha256.Sum256([]byte(st.PKCEVerifier))
	expectedChallenge := hex.EncodeToString(sum[:])
	require.Equal(t, expectedChallenge, st.PKCEChallenge)
	require.WithinDuration(t, time.Now().UTC(), st.CreatedAt, 2*time.Second)
}

func TestInitProviderAuth_UnsupportedProvider(t *testing.T) {
	svc := authn.NewOAuthProviderService(authn.ProvidersConfig{}, authn.NewMemoryInitStateStore())
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	t.Cleanup(cancel)
	_, err := svc.InitProviderAuth(ctx, authnpb.AuthProvider_AUTH_PROVIDER_UNSPECIFIED)
	require.ErrorIs(t, err, authn.ErrProviderUnsupported, "expected ErrProviderUnsupported; got: %v", err)
}
