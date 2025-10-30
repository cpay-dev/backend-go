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

func startTestServer(t *testing.T) (addr string, stop func()) {
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
	srv := authn.NewServer(authn.ServerConfig{AuthService: authn.NewMockAuthService(map[authnpb.AuthProvider]string{
		authnpb.AuthProvider_AUTH_PROVIDER_GOOGLE: mockURL,
	})})
	srv.Register(grpcServer)
	srv.MarkReady()

	go func() {
		assert.NoError(t, grpcServer.Serve(lis), "grpc: serve")
	}()

	return lis.Addr().String(), func() { srv.Shutdown(grpcServer) }
}

func TestHealthChecks(t *testing.T) {
	t.Parallel()

	addr, stop := startTestServer(t)
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

	addr, stop := startTestServer(t)
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
	require.Equal(t, mockURL, resp.GetAuthUrl(), "should return mock URL")
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
