package authn

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/IndexStorm/ulid"
	authnpb "github.com/cpay-dev/proto-go/api/v1/authn"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type AuthService interface {
	AuthURL(ctx context.Context, provider authnpb.AuthProvider, nonce, redirectPath string) (string, error)
}

var (
	// ErrProviderUnsupported is returned when the provider is not supported by the service.
	ErrProviderUnsupported = errors.New("provider not supported")
)

// OAuthInitState captures parameters for an OAuth initiation flow.
type OAuthInitState struct {
	ID           string
	Provider     authnpb.AuthProvider
	Nonce        string
	RedirectPath string
	CreatedAt    time.Time
}

const initStateTTL = 15 * time.Minute

type OAuthService struct {
	providers map[authnpb.AuthProvider]oauth2.Config
	store     InitStateStore
}

func NewOAuthService(p ProvidersConfig, store InitStateStore) *OAuthService {
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
	return &OAuthService{providers: providers, store: store}
}

func (s *OAuthService) AuthURL(ctx context.Context, provider authnpb.AuthProvider, nonce, redirectPath string) (string, error) {
	conf, ok := s.providers[provider]
	if !ok {
		return "", ErrProviderUnsupported
	}

	// Generate a ULID to serve as the OAuth state parameter and storage key suffix.
	stateID := ulid.Make().String()

	// Persist the init state for later validation on callback.
	initState := OAuthInitState{
		ID:           stateID,
		Provider:     provider,
		Nonce:        nonce,
		RedirectPath: redirectPath,
		CreatedAt:    time.Now().UTC(),
	}
	if err := s.store.Save(ctx, "authn:init:"+stateID, initState, initStateTTL); err != nil {
		return "", fmt.Errorf("save init state: %w", err)
	}

	url := conf.AuthCodeURL(
		stateID,
		oauth2.AccessTypeOffline,
	)
	return url, nil
}
