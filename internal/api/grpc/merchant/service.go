package merchant

import (
	apiasset "github.com/cpay-dev/backend-go/internal/api/asset"
	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant/asset"
	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant/chain"
	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant/payment"
	pgblockchain "github.com/cpay-dev/backend-go/internal/api/repo/pg/blockchain"
	pgpayment "github.com/cpay-dev/backend-go/internal/api/repo/pg/payment"
	apiwallet "github.com/cpay-dev/backend-go/internal/api/wallet"
	pbasset "github.com/cpay-dev/proto-go/api/v1/merchant/asset"
	pbchain "github.com/cpay-dev/proto-go/api/v1/merchant/chain"
	pbpayment "github.com/cpay-dev/proto-go/api/v1/merchant/payment"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
)

type Service struct {
	pbasset.UnsafeAssetServiceServer
	pbchain.UnsafeChainServiceServer
	pbpayment.UnsafePaymentServiceServer

	assetService   *asset.Service
	chainService   *chain.Service
	paymentService *payment.Service

	healthServer *health.Server
}

func NewService(
	priceService *apiasset.PriceService,
	blockchainRepo *pgblockchain.PostgresRepo,
	paymentRepo *pgpayment.PostgresRepo,
	walletService *apiwallet.Service,
) *Service {
	return &Service{
		assetService:   asset.NewService(blockchainRepo, priceService),
		chainService:   chain.NewService(blockchainRepo),
		paymentService: payment.NewService(blockchainRepo, paymentRepo, walletService),
		healthServer:   health.NewServer(),
	}
}

func (s *Service) Bind(server *grpc.Server) {
	pbasset.RegisterAssetServiceServer(server, s.assetService)
	pbchain.RegisterChainServiceServer(server, s.chainService)
	pbpayment.RegisterPaymentServiceServer(server, s.paymentService)
}
