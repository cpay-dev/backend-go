package authn

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/cpay-dev/backend-go/pkg/crypto"
	"github.com/cpay-dev/backend-go/pkg/pkce"
	authnpb "github.com/cpay-dev/proto-go/api/v1/authn"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const initStateTTL = 15 * time.Minute

var (
	// ErrProviderUnsupported is returned when the provider is not supported by the service.
	ErrProviderUnsupported = errors.New("provider not supported")
	// ErrIDTokenInvalid is returned when the ID token verification or claims validation fails.
	ErrIDTokenInvalid = errors.New("id token invalid")
)

type AuthService interface {
	InitProviderAuth(ctx context.Context, provider authnpb.AuthProvider) (InitAuthResult, error)
	ContinueProviderAuth(ctx context.Context, state, code string) (ProviderCallbackResult, error)
}

// InitAuthResult contains values needed by clients to continue auth with a provider.
type InitAuthResult struct {
	State       string
	RedirectURL string
}

// ProviderCallbackResult contains user identity information extracted from provider callback.
type ProviderCallbackResult struct {
	Email         string
	EmailVerified bool
}

type OAuthProviderService struct {
	providers map[authnpb.AuthProvider]oauth2.Config
	store     InitStateStore
	jwks      JWKSService
	parsers   map[authnpb.AuthProvider]*jwt.Parser
}

func NewOAuthProviderService(p ProvidersConfig, store InitStateStore, jwks JWKSService) *OAuthProviderService {
	if store == nil {
		panic("InitStateStore is required")
	}
	if jwks == nil {
		panic("JWKSService is required")
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
	parsers := map[authnpb.AuthProvider]*jwt.Parser{
		authnpb.AuthProvider_AUTH_PROVIDER_GOOGLE: jwt.NewParser(
			jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Name}),
			jwt.WithIssuer("https://accounts.google.com"),
			jwt.WithAudience(p.Google.ClientID),
			jwt.WithExpirationRequired(),
			jwt.WithIssuedAt(),
			jwt.WithLeeway(time.Second*10),
			jwt.WithStrictDecoding(),
		),
	}
	return &OAuthProviderService{providers: providers, store: store, jwks: jwks, parsers: parsers}
}

func (s *OAuthProviderService) InitProviderAuth(ctx context.Context, provider authnpb.AuthProvider) (InitAuthResult, error) {
	conf, ok := s.providers[provider]
	if !ok {
		return InitAuthResult{}, ErrProviderUnsupported
	}

	state := crypto.RandomULID().String()
	nonce := crypto.RandomULID().String()
	verifier, challenge := pkce.GenerateStateS256()

	initState := OAuthInitState{
		ID:            state,
		Provider:      provider,
		Nonce:         nonce,
		PKCEVerifier:  verifier,
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

func (s *OAuthProviderService) ContinueProviderAuth(ctx context.Context, state, code string) (ProviderCallbackResult, error) {
	code, err := url.QueryUnescape(code)
	if err != nil {
		return ProviderCallbackResult{}, fmt.Errorf("unescape code: %w", err)
	}

	initState, err := s.store.Pop(ctx, "authn:init:"+state)
	if err != nil {
		return ProviderCallbackResult{}, fmt.Errorf("pop init state: %w", err)
	}

	conf, ok := s.providers[initState.Provider]
	if !ok {
		return ProviderCallbackResult{}, ErrProviderUnsupported
	}

	switch initState.Provider {
	case authnpb.AuthProvider_AUTH_PROVIDER_GOOGLE:
		return s.continueGoogleProviderAuth(ctx, conf, initState, code)
	default:
		return ProviderCallbackResult{}, ErrProviderUnsupported
	}
}

func (s *OAuthProviderService) continueGoogleProviderAuth(ctx context.Context, conf oauth2.Config, initState OAuthInitState, code string) (ProviderCallbackResult, error) {
	parser, ok := s.parsers[authnpb.AuthProvider_AUTH_PROVIDER_GOOGLE]
	if !ok || parser == nil {
		return ProviderCallbackResult{}, ErrProviderUnsupported
	}

	tok, err := conf.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", initState.PKCEVerifier))
	if err != nil {
		return ProviderCallbackResult{}, fmt.Errorf("exchange code: %w", err)
	}

	rawIDToken, _ := tok.Extra("id_token").(string)
	if rawIDToken == "" {
		return ProviderCallbackResult{}, ErrIDTokenInvalid
	}

	type GoogleIDTokenClaims struct {
		jwt.RegisteredClaims
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Nonce         string `json:"nonce"`
	}
	var claims GoogleIDTokenClaims

	keyfunc := s.jwks.Keyfunc()
	if keyfunc == nil {
		return ProviderCallbackResult{}, ErrIDTokenInvalid
	}

	if _, err = parser.ParseWithClaims(rawIDToken, &claims, keyfunc); err != nil {
		return ProviderCallbackResult{}, ErrIDTokenInvalid
	}

	if claims.Nonce != initState.Nonce {
		return ProviderCallbackResult{}, ErrIDTokenInvalid
	}

	email := claims.Email
	emailVerified := claims.EmailVerified

	return ProviderCallbackResult{Email: email, EmailVerified: emailVerified}, nil
}
