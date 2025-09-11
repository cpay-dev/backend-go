package merchant

import (
	pgblockchain "github.com/cpay-dev/backend-go/internal/api/repo/pg/blockchain"
	pbmerchant "github.com/cpay-dev/proto-go/api/v1/merchant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
)

type Service struct {
	pbmerchant.UnsafeMerchantServiceServer
	healthServer *health.Server
	repo         *pgblockchain.PostgresRepo
}

func NewService(
	repo *pgblockchain.PostgresRepo,
) *Service {
	return &Service{
		repo:         repo,
		healthServer: health.NewServer(),
	}
}

func (s *Service) Bind(server *grpc.Server) {
	pbmerchant.RegisterMerchantServiceServer(server, s)
}
