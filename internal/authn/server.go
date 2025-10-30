package authn

import (
	"github.com/cpay-dev/backend-go/pkg/grpcmw"
	authnpb "github.com/cpay-dev/proto-go/api/v1/authn"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type Server struct {
	authnpb.UnimplementedAuthnServiceServer
	auth      AuthService
	healthSrv *health.Server
	logger    zerolog.Logger
}

func NewServer(cfg ServerConfig) *Server {
	if cfg.AuthService == nil {
		panic("AuthService is required")
	}
	return &Server{
		auth:      cfg.AuthService,
		logger:    cfg.Logger,
		healthSrv: health.NewServer(),
	}
}

func DefaultMiddleware() []grpc.UnaryServerInterceptor {
	return []grpc.UnaryServerInterceptor{
		grpcmw.UnaryCore(),
		grpcmw.UnaryPanicRecover(),
		grpcmw.UnaryRequestLogger(),
	}
}

func (s *Server) Register(gs *grpc.Server) {
	healthpb.RegisterHealthServer(gs, s.healthSrv)
	authnpb.RegisterAuthnServiceServer(gs, s)
}

func (s *Server) MarkReady() {
	s.logger.Debug().Msg("health: set SERVING")
	s.healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	s.healthSrv.SetServingStatus("cpay.api.v1.authn.AuthnService", healthpb.HealthCheckResponse_SERVING)
}

func (s *Server) MarkNotReady() {
	s.logger.Debug().Msg("health: set NOT_SERVING")
	s.healthSrv.Shutdown()
}

func (s *Server) Shutdown(gs *grpc.Server) {
	s.logger.Info().Msg("shutdown: graceful stop")
	s.MarkNotReady()
	gs.GracefulStop()
}
