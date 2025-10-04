package merchant

import (
	"fmt"
	"net"
	"time"

	"github.com/cpay-dev/backend-go/internal/api/authn"
	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant/asset"
	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant/chain"
	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant/middleware"
	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant/payment"
	pkgmw "github.com/cpay-dev/backend-go/pkg/grpc/middleware"
	pbasset "github.com/cpay-dev/proto-go/api/v1/merchant/asset"
	pbchain "github.com/cpay-dev/proto-go/api/v1/merchant/chain"
	pbpayment "github.com/cpay-dev/proto-go/api/v1/merchant/payment"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

type Server struct {
	server         *grpc.Server
	assetService   *asset.Service
	chainService   *chain.Service
	paymentService *payment.Service
	healthService  *healthService
}

func NewServer(
	logger zerolog.Logger,
	authnService *authn.AuthnService,
	assetService *asset.Service,
	chainService *chain.Service,
	paymentService *payment.Service,
) *Server {
	return &Server{
		assetService:   assetService,
		chainService:   chainService,
		paymentService: paymentService,
		healthService:  newHealthService(logger, assetService, chainService, paymentService),
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
				pkgmw.NewCore(logger, pkgmw.DefaultHealthBypass),
				pkgmw.NewApiKey(pkgmw.DefaultHealthBypass),
				middleware.NewMerchant(authnService, pkgmw.DefaultHealthBypass),
			),
		),
	}
}

func (s *Server) Start(listenAddress string, healthProbe time.Duration) error {
	pbasset.RegisterAssetServiceServer(s.server, s.assetService)
	pbchain.RegisterChainServiceServer(s.server, s.chainService)
	pbpayment.RegisterPaymentServiceServer(s.server, s.paymentService)
	s.healthService.Bind(s.server)

	lis, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	if err := s.healthService.Start(healthProbe); err != nil {
		return fmt.Errorf("start health service: %w", err)
	}

	if err = s.server.Serve(lis); err != nil {
		return fmt.Errorf("serve: %w", err)
	}

	return nil
}

func (s *Server) Stop() {
	if s.healthService != nil {
		s.healthService.Stop()
	}
	s.server.GracefulStop()
}
