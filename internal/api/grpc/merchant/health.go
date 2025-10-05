package merchant

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant/asset"
	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant/chain"
	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant/payment"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type healthService struct {
	doneCh         chan struct{}
	server         *health.Server
	assetService   *asset.Service
	chainService   *chain.Service
	paymentService *payment.Service
	logger         zerolog.Logger
}

func newHealthService(
	logger zerolog.Logger,
	assetService *asset.Service,
	chainService *chain.Service,
	paymentService *payment.Service,
) *healthService {
	return &healthService{
		doneCh:         make(chan struct{}),
		logger:         logger,
		server:         health.NewServer(),
		assetService:   assetService,
		chainService:   chainService,
		paymentService: paymentService,
	}
}

func (s *healthService) Bind(server *grpc.Server) {
	grpc_health_v1.RegisterHealthServer(server, s.server)
}

func (s *healthService) Start(interval time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	if err := s.CheckHealth(ctx); err != nil {
		return err
	}
	s.server.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	go s.run(interval)
	return nil
}

func (s *healthService) CheckHealth(ctx context.Context) error {
	checks := []func(ctx context.Context) error{
		s.CheckAssetHealth,
		s.CheckChainHealth,
		s.CheckPaymentHealth,
	}

	wg := sync.WaitGroup{}
	wg.Add(len(checks))
	errCh := make(chan error, len(checks))

	for _, check := range checks {
		go func() {
			defer wg.Done()
			if err := check(ctx); err != nil {
				errCh <- err
			}
		}()
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func (s *healthService) Stop() {
	close(s.doneCh)
	s.server.Shutdown()
}

func (s *healthService) run(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for {
		select {
		case <-s.doneCh:
			return
		case <-ticker.C:
			s.checkHealth()
		}
	}
}

func (s *healthService) checkHealth() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	if err := s.CheckHealth(ctx); err != nil {
		s.logger.Err(err).Msg("service is not healthy")
	}
}

func (s *healthService) CheckAssetHealth(ctx context.Context) error {
	var err error = nil
	if err != nil {
		s.server.SetServingStatus("asset", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	} else {
		s.server.SetServingStatus("asset", grpc_health_v1.HealthCheckResponse_SERVING)
	}
	return err
}

func (s *healthService) CheckChainHealth(ctx context.Context) error {
	var err error = nil
	if err != nil {
		s.server.SetServingStatus("chain", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	} else {
		s.server.SetServingStatus("chain", grpc_health_v1.HealthCheckResponse_SERVING)
	}
	return err
}

func (s *healthService) CheckPaymentHealth(ctx context.Context) error {
	var err error = nil
	if err != nil {
		s.server.SetServingStatus("payment", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	} else {
		s.server.SetServingStatus("payment", grpc_health_v1.HealthCheckResponse_SERVING)
	}
	return err
}
