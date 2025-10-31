package authn

import (
	"fmt"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/imroc/req/v3"
)

// JWKSService provides JWT verification via remote JWKS.
type JWKSService interface {
	// Keyfunc returns a jwt.Keyfunc for token verification.
	Keyfunc() jwt.Keyfunc
	// Bootstrap performs an initial blocking fetch of keys with the given timeout.
	Bootstrap(timeout time.Duration) error
}

type KeyfuncJWKSService struct {
	urls   []string
	client *req.Client
	kf     keyfunc.Keyfunc
}

// NewKeyfuncJWKSService constructs a JWKS service using the given URLs and req client.
// If client is nil, a default req client is used.
func NewKeyfuncJWKSService(urls []string, client *req.Client) *KeyfuncJWKSService {
	return &KeyfuncJWKSService{urls: urls, client: client}
}

func (s *KeyfuncJWKSService) Keyfunc() jwt.Keyfunc {
	if s.kf == nil {
		return nil
	}
	return s.kf.Keyfunc
}

func (s *KeyfuncJWKSService) Bootstrap(timeout time.Duration) error {
	httpURLs := make(map[string]jwkset.Storage)
	for _, u := range s.urls {
		st, err := jwkset.NewStorageFromHTTP(u, jwkset.HTTPClientStorageOptions{
			Client:          s.client.GetClient(),
			HTTPTimeout:     timeout,
			RefreshInterval: time.Hour,
		})
		if err != nil {
			return fmt.Errorf("jwks storage init for %s: %w", u, err)
		}
		httpURLs[u] = st
	}

	storage, err := jwkset.NewHTTPClient(jwkset.HTTPClientOptions{HTTPURLs: httpURLs})
	if err != nil {
		return fmt.Errorf("jwks http client: %w", err)
	}
	kf, err := keyfunc.New(keyfunc.Options{Storage: storage})
	if err != nil {
		return fmt.Errorf("jwks keyfunc: %w", err)
	}
	s.kf = kf
	return nil
}
