package authn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/IndexStorm/ulid"
	authnpb "github.com/cpay-dev/proto-go/api/v1/authn"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const initStateTTL = 15 * time.Minute

var (
	// ErrProviderUnsupported is returned when the provider is not supported by the service.
	ErrProviderUnsupported = errors.New("provider not supported")
)

type AuthService interface {
	InitProviderAuth(ctx context.Context, provider authnpb.AuthProvider) (InitAuthResult, error)
}

// InitAuthResult contains values needed by clients to continue auth with a provider.
type InitAuthResult struct {
	State       string
	RedirectURL string
}

type OAuthProviderService struct {
	providers map[authnpb.AuthProvider]oauth2.Config
	store     InitStateStore
}

func NewOAuthProviderService(p ProvidersConfig, store InitStateStore) *OAuthProviderService {
	if store == nil {
		panic("InitStateStore is required")
	}
	providers := map[authnpb.AuthProvider]oauth2.Config{
		authnpb.AuthProvider_AUTH_PROVIDER_GOOGLE: {
			ClientID:     p.Google.ClientID,
			ClientSecret: p.Google.ClientSecret,
			RedirectURL:  p.Google.RedirectURI,
			Scopes:       p.Google.Scopes,
			Endpoint:     google.Endpoint,
		},
	}
	return &OAuthProviderService{providers: providers, store: store}
}

func (s *OAuthProviderService) InitProviderAuth(ctx context.Context, provider authnpb.AuthProvider) (InitAuthResult, error) {
	conf, ok := s.providers[provider]
	if !ok {
		return InitAuthResult{}, ErrProviderUnsupported
	}

	state := ulid.Make().String()
	nonce := ulid.Make().String()
	pkceVerifier := ulid.Make().String()
	pkceChallenge := sha256.Sum256([]byte(pkceVerifier))
	challenge := hex.EncodeToString(pkceChallenge[:])

	initState := OAuthInitState{
		ID:            state,
		Provider:      provider,
		Nonce:         nonce,
		PKCEVerifier:  pkceVerifier,
		PKCEChallenge: challenge,
		CreatedAt:     time.Now().UTC(),
	}
	if err := s.store.Save(ctx, "authn:init:"+state, initState, initStateTTL); err != nil {
		return InitAuthResult{}, fmt.Errorf("save init state: %w", err)
	}

	url := conf.AuthCodeURL(
		state,
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)

	return InitAuthResult{State: state, RedirectURL: url}, nil
}
