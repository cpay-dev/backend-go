package merchant

import (
	pgblockchain "github.com/cpay-dev/backend-go/internal/api/repo/pg/blockchain"
	pgpayment "github.com/cpay-dev/backend-go/internal/api/repo/pg/payment"
	pbmerchant "github.com/cpay-dev/proto-go/api/v1/merchant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
)

type Service struct {
	pbmerchant.UnsafeAssetServiceServer
	pbmerchant.UnsafeChainServiceServer
	pbmerchant.UnsafePaymentIntentServiceServer

	healthServer   *health.Server
	blockchainRepo *pgblockchain.PostgresRepo
	paymentRepo    *pgpayment.PostgresRepo
}

func NewService(
	blockchainRepo *pgblockchain.PostgresRepo,
	paymentRepo *pgpayment.PostgresRepo,
) *Service {
	return &Service{
		blockchainRepo: blockchainRepo,
		paymentRepo:    paymentRepo,
		healthServer:   health.NewServer(),
	}
}

func (s *Service) Bind(server *grpc.Server) {
	pbmerchant.RegisterAssetServiceServer(server, s)
	pbmerchant.RegisterChainServiceServer(server, s)
	pbmerchant.RegisterPaymentIntentServiceServer(server, s)
}
