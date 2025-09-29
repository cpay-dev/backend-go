package merchant

import (
	"context"
	"fmt"
	"time"

	pbmerchant "github.com/cpay-dev/proto-go/api/v1/merchant"
	pbblockchain "github.com/cpay-dev/proto-go/blockchain/v1"
	"google.golang.org/grpc/health/grpc_health_v1"
)

func (s *Server) CheckHealth() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	_, err := s.service.ListAssets(ctx, &pbmerchant.ListAssetsRequest{ChainId: pbblockchain.Chain_CHAIN_ANY})
	if err != nil {
		return fmt.Errorf("check health: %w", err)
	}
	return nil
}

func (s *Server) updateReadiness() {
	if err := s.CheckHealth(); err != nil {
		s.healthServer.SetServingStatus("merchant", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
		s.logger.Err(err).Msg("merchant service is unhealthy")
	} else {
		s.healthServer.SetServingStatus("merchant", grpc_health_v1.HealthCheckResponse_SERVING)
	}
}
