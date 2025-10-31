package authn

import (
	"context"

	authnpb "github.com/cpay-dev/proto-go/api/v1/authn"
)

// NewMockAuthService constructs a MockAuthService with the given provider URLs.
func NewMockAuthService(urls map[authnpb.AuthProvider]string) MockAuthService {
	return MockAuthService{URLs: urls}
}

// MockAuthService is a simple in-memory implementation of AuthService for tests.
// It returns URLs from the provided map or ErrProviderUnsupported when missing.
type MockAuthService struct {
	URLs map[authnpb.AuthProvider]string
	Err  error
}

func (m MockAuthService) InitProviderAuth(_ context.Context, p authnpb.AuthProvider) (InitAuthResult, error) {
	if m.Err != nil {
		return InitAuthResult{}, m.Err
	}
	if m.URLs != nil {
		if u, ok := m.URLs[p]; ok && u != "" {
			return InitAuthResult{State: "mock-state", RedirectURL: u}, nil
		}
	}
	return InitAuthResult{}, ErrProviderUnsupported
}
