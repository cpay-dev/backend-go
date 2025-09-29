package merchant

import (
	"fmt"
	"net"
	"time"

	"github.com/cpay-dev/backend-go/internal/api/authn"
	pgblockchain "github.com/cpay-dev/backend-go/internal/api/repo/pg/blockchain"
	pgpayment "github.com/cpay-dev/backend-go/internal/api/repo/pg/payment"
	"github.com/cpay-dev/backend-go/pkg/grpc/middleware"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
)

type Server struct {
	service      *Service
	server       *grpc.Server
	healthServer *health.Server
	healthTicker *time.Ticker
	healthStop   chan struct{}
	logger       zerolog.Logger
}

func NewServer(
	logger zerolog.Logger,
	blockchainRepo *pgblockchain.PostgresRepo,
	paymentRepo *pgpayment.PostgresRepo,
	authnService *authn.AuthnService,
	healthInterval time.Duration,
) *Server {
	return &Server{
		logger:       logger,
		service:      NewService(blockchainRepo, paymentRepo),
		healthServer: health.NewServer(),
		healthTicker: time.NewTicker(healthInterval),
		healthStop:   make(chan struct{}),
		server: grpc.NewServer(
			grpc.SharedWriteBuffer(true),
			grpc.KeepaliveParams(keepalive.ServerParameters{
				MaxConnectionIdle: time.Minute,
				Time:              time.Second * 30,
				Timeout:           time.Second * 10,
			}),
			grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
				MinTime: time.Second * 15,
			}),
			grpc.ConnectionTimeout(time.Second*15),
			grpc.WaitForHandlers(true),
			grpc.ChainUnaryInterceptor(
				middleware.NewCore(logger, middleware.DefaultHealthBypass),
				middleware.NewApiKey(middleware.DefaultHealthBypass),
				newMerchantMiddleware(authnService, middleware.DefaultHealthBypass),
			),
		),
	}
}

func (s *Server) Start(listenAddress string) error {
	s.service.Bind(s.server)
	grpc_health_v1.RegisterHealthServer(s.server, s.healthServer)

	lis, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	s.healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	s.updateReadiness()

	go func() {
		for {
			select {
			case <-s.healthStop:
				return
			case <-s.healthTicker.C:
				s.updateReadiness()
			}
		}
	}()

	if err = s.server.Serve(lis); err != nil {
		return fmt.Errorf("serve: %w", err)
	}

	return nil
}

func (s *Server) Stop() {
	close(s.healthStop)
	s.healthTicker.Stop()
	s.healthServer.Shutdown()
	s.server.GracefulStop()
}
