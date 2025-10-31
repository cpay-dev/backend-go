package authn_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	authnpb "github.com/cpay-dev/proto-go/api/v1/authn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	"github.com/cpay-dev/backend-go/internal/authn"
)

const mockURL = "http://mock/auth"

type panicSrv struct {
	authnpb.UnimplementedAuthnServiceServer
}

func (panicSrv) InitAuth(ctx context.Context, _ *authnpb.InitAuthRequest) (*authnpb.InitAuthResponse, error) {
	panic("boom")
}

func startTestServer(t *testing.T, authService authn.AuthService) (addr string, stop func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "listen on ephemeral port")
	t.Cleanup(func() {
		err := lis.Close()
		if err != nil && !errors.Is(err, net.ErrClosed) {
			assert.NoError(t, err, "close listener")
		}
	})

	grpcServer := grpc.NewServer(grpc.ChainUnaryInterceptor(authn.DefaultMiddleware()...))
	if authService == nil {
		authService = authn.NewMockAuthService(map[authnpb.AuthProvider]string{
			authnpb.AuthProvider_AUTH_PROVIDER_GOOGLE: mockURL,
		})
	}
	srv := authn.NewServer(authn.ServerConfig{AuthService: authService})
	srv.Register(grpcServer)
	srv.MarkReady()

	go func() {
		assert.NoError(t, grpcServer.Serve(lis), "grpc: serve")
	}()

	return lis.Addr().String(), func() { srv.Shutdown(grpcServer) }
}

func TestHealthChecks(t *testing.T) {
	t.Parallel()

	addr, stop := startTestServer(t, nil)
	t.Cleanup(stop)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err, "dial %s", addr)
	t.Cleanup(func() { assert.NoError(t, conn.Close(), "close conn") })

	hc := healthpb.NewHealthClient(conn)
	// Liveness
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	t.Cleanup(cancel)
	r1, err := hc.Check(ctx, &healthpb.HealthCheckRequest{Service: ""})
	require.NoError(t, err, "health liveness check")
	require.Equal(t, healthpb.HealthCheckResponse_SERVING, r1.Status, "liveness should be SERVING")
	// Readiness
	r2, err := hc.Check(ctx, &healthpb.HealthCheckRequest{Service: "cpay.api.v1.authn.AuthnService"})
	require.NoError(t, err, "health readiness check")
	require.Equal(t, healthpb.HealthCheckResponse_SERVING, r2.Status, "readiness should be SERVING")
}

func TestInitAuthProviderGoogle(t *testing.T) {
	t.Parallel()

	addr, stop := startTestServer(t, nil)
	t.Cleanup(stop)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err, "dial %s", addr)
	t.Cleanup(func() { assert.NoError(t, conn.Close(), "close conn") })

	client := authnpb.NewAuthnServiceClient(conn)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	t.Cleanup(cancel)
	resp, err := client.InitAuth(ctx, &authnpb.InitAuthRequest{
		Method: &authnpb.InitAuthRequest_Provider{
			Provider: &authnpb.ProviderMethod{Provider: authnpb.AuthProvider_AUTH_PROVIDER_GOOGLE},
		},
	})
	require.NoError(t, err, "InitAuth google should succeed")
	pc := resp.GetProvider()
	require.NotNil(t, pc, "provider continuation must be present")
	require.Equal(t, mockURL, pc.GetRedirectUrl(), "should return mock URL")
	require.NotEmpty(t, pc.GetState(), "state must be present")
}

func TestPanicRecovered(t *testing.T) {
	t.Parallel()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "listen for panic-recovery server")
	t.Cleanup(func() {
		err := lis.Close()
		if err != nil && !errors.Is(err, net.ErrClosed) {
			assert.NoError(t, err, "close listener")
		}
	})

	gs := grpc.NewServer(grpc.ChainUnaryInterceptor(authn.DefaultMiddleware()...))
	t.Cleanup(gs.GracefulStop)

	authnpb.RegisterAuthnServiceServer(gs, panicSrv{})
	go func() {
		assert.NoError(t, gs.Serve(lis), "grpc: serve")
	}()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err, "dial panic-recovery server")
	t.Cleanup(func() { assert.NoError(t, conn.Close(), "close conn") })
	client := authnpb.NewAuthnServiceClient(conn)
	_, err = client.InitAuth(t.Context(), &authnpb.InitAuthRequest{Method: &authnpb.InitAuthRequest_Provider{Provider: &authnpb.ProviderMethod{}}})
	st, ok := status.FromError(err)
	require.True(t, ok, "error must be a status error")
	require.Equal(t, codes.Internal, st.Code(), "recovered panic should map to Internal")
	require.Empty(t, st.Message(), "error message must be empty")
}

func TestContinueAuthProviderCallbackSuccess(t *testing.T) {
	t.Parallel()

	mock := authn.NewMockAuthService(map[authnpb.AuthProvider]string{})
	mock = authn.MockAuthService{
		URLs:        mock.URLs,
		Err:         nil,
		ContinueRes: authn.ProviderCallbackResult{Email: "user@example.com", EmailVerified: true},
	}

	addr, stop := startTestServer(t, mock)
	t.Cleanup(stop)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err, "dial %s", addr)
	t.Cleanup(func() { assert.NoError(t, conn.Close(), "close conn") })

	client := authnpb.NewAuthnServiceClient(conn)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	t.Cleanup(cancel)

	resp, err := client.ContinueAuth(ctx, &authnpb.ContinueAuthRequest{
		Method: &authnpb.ContinueAuthRequest_ProviderCallback{
			ProviderCallback: &authnpb.ProviderCallbackMethod{State: "s", Code: "c"},
		},
	})
	require.NoError(t, err, "ContinueAuth provider callback should succeed")
	pd := resp.GetProviderData()
	require.NotNil(t, pd, "provider data must be present")
	require.Equal(t, "user@example.com", pd.GetEmail())
	require.True(t, pd.GetEmailVerified())
}

func TestContinueAuth_InvalidState_ReturnsInvalidArgument(t *testing.T) {
	t.Parallel()

	mock := authn.MockAuthService{Err: authn.ErrStateNotFound}
	addr, stop := startTestServer(t, mock)
	t.Cleanup(stop)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err, "dial %s", addr)
	t.Cleanup(func() { assert.NoError(t, conn.Close(), "close conn") })

	client := authnpb.NewAuthnServiceClient(conn)
	_, err = client.ContinueAuth(t.Context(), &authnpb.ContinueAuthRequest{
		Method: &authnpb.ContinueAuthRequest_ProviderCallback{
			ProviderCallback: &authnpb.ProviderCallbackMethod{State: "missing", Code: "code"},
		},
	})
	st, ok := status.FromError(err)
	require.True(t, ok, "error must be a status error")
	require.Equal(t, codes.InvalidArgument, st.Code(), "invalid state should map to InvalidArgument")
}

func TestContinueAuth_Unimplemented_WhenMissingMethod(t *testing.T) {
	t.Parallel()

	addr, stop := startTestServer(t, nil)
	t.Cleanup(stop)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err, "dial %s", addr)
	t.Cleanup(func() { assert.NoError(t, conn.Close(), "close conn") })

	client := authnpb.NewAuthnServiceClient(conn)
	_, err = client.ContinueAuth(t.Context(), &authnpb.ContinueAuthRequest{})
	st, ok := status.FromError(err)
	require.True(t, ok, "error must be a status error")
	require.Equal(t, codes.Unimplemented, st.Code(), "missing method should map to Unimplemented")
}
