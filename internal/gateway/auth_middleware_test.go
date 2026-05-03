package gateway

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/shared/ids"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type authMiddlewareTestServer struct {
	cpayv1.UnimplementedAuthServiceServer
	validateFn               func(context.Context, *cpayv1.ValidateCredentialRequest) (*cpayv1.ValidateCredentialResponse, error)
	getMerchantSettingsFn    func(context.Context, *cpayv1.GetMerchantSettingsRequest) (*cpayv1.GetMerchantSettingsResponse, error)
	updateMerchantSettingsFn func(context.Context, *cpayv1.UpdateMerchantSettingsRequest) (*cpayv1.UpdateMerchantSettingsResponse, error)
}

func (s *authMiddlewareTestServer) ValidateCredential(ctx context.Context, req *cpayv1.ValidateCredentialRequest) (*cpayv1.ValidateCredentialResponse, error) {
	return s.validateFn(ctx, req)
}

func (s *authMiddlewareTestServer) GetMerchantSettings(ctx context.Context, req *cpayv1.GetMerchantSettingsRequest) (*cpayv1.GetMerchantSettingsResponse, error) {
	return s.getMerchantSettingsFn(ctx, req)
}

func (s *authMiddlewareTestServer) UpdateMerchantSettings(ctx context.Context, req *cpayv1.UpdateMerchantSettingsRequest) (*cpayv1.UpdateMerchantSettingsResponse, error) {
	return s.updateMerchantSettingsFn(ctx, req)
}

func newAuthTestClient(t *testing.T, srv cpayv1.AuthServiceServer) cpayv1.AuthServiceClient {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	cpayv1.RegisterAuthServiceServer(grpcServer, srv)

	go func() {
		_ = grpcServer.Serve(listener)
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})

	conn, err := grpc.DialContext(
		context.Background(),
		"bufnet",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return cpayv1.NewAuthServiceClient(conn)
}

func TestAuthnRequiresCredentials(t *testing.T) {
	s := &Server{
		authClient: newAuthTestClient(t, &authMiddlewareTestServer{
			validateFn: func(context.Context, *cpayv1.ValidateCredentialRequest) (*cpayv1.ValidateCredentialResponse, error) {
				t.Fatal("ValidateCredential should not be called for missing credentials")
				return nil, nil
			},
		}),
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	rr := httptest.NewRecorder()

	called := false
	handler := s.authn(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	handler.ServeHTTP(rr, req)

	if called {
		t.Fatalf("next handler should not be called")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestAuthnSetsRequesterFromValidatedPrincipal(t *testing.T) {
	var gotAuthorization string
	var gotAPIKey string
	client := newAuthTestClient(t, &authMiddlewareTestServer{
		validateFn: func(_ context.Context, req *cpayv1.ValidateCredentialRequest) (*cpayv1.ValidateCredentialResponse, error) {
			gotAuthorization = req.GetAuthorization()
			gotAPIKey = req.GetApiKey()
			return &cpayv1.ValidateCredentialResponse{
				Principal: &cpayv1.Principal{
					MerchantId: ids.New(),
					UserId:     ids.New(),
					Role:       "admin",
					IsUser:     true,
				},
			}, nil
		},
	})
	s := &Server{authClient: client}

	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rr := httptest.NewRecorder()

	called := false
	handler := s.authn(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		parsed, ok := requesterFromContext(r.Context())
		if !ok {
			t.Fatalf("requester missing from context")
		}
		if parsed.Role != "admin" {
			t.Fatalf("expected role admin, got %q", parsed.Role)
		}
		if parsed.UserID == nil {
			t.Fatalf("expected user_id in requester context")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	handler.ServeHTTP(rr, req)

	if !called {
		t.Fatalf("next handler was not called")
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
	if gotAuthorization != "Bearer test-token" {
		t.Fatalf("expected authorization to be forwarded, got %q", gotAuthorization)
	}
	if gotAPIKey != "" {
		t.Fatalf("expected empty api key, got %q", gotAPIKey)
	}
}

func TestAuthnRejectsInvalidPrincipalMerchant(t *testing.T) {
	client := newAuthTestClient(t, &authMiddlewareTestServer{
		validateFn: func(_ context.Context, _ *cpayv1.ValidateCredentialRequest) (*cpayv1.ValidateCredentialResponse, error) {
			return &cpayv1.ValidateCredentialResponse{
				Principal: &cpayv1.Principal{
					MerchantId: "invalid-merchant-id",
					Role:       "admin",
					IsUser:     true,
				},
			}, nil
		},
	})
	s := &Server{authClient: client}

	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rr := httptest.NewRecorder()

	handler := s.authn(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatalf("next handler should not be called for invalid principal")
	}))
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}
