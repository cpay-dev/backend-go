package payment

import (
	apiasset "github.com/cpay-dev/backend-go/internal/api/asset"
	pgblockchain "github.com/cpay-dev/backend-go/internal/api/repo/pg/blockchain"
	pgpayment "github.com/cpay-dev/backend-go/internal/api/repo/pg/payment"
	pbpayment "github.com/cpay-dev/proto-go/api/v1/merchant/payment"
)

var _ pbpayment.PaymentServiceServer = (*Service)(nil)

type Service struct {
	pbpayment.UnsafePaymentServiceServer
	blockchainRepo *pgblockchain.PostgresRepo
	paymentRepo    *pgpayment.PostgresRepo
	priceService   *apiasset.PriceService
}

func NewService(
	blockchainRepo *pgblockchain.PostgresRepo,
	paymentRepo *pgpayment.PostgresRepo,
) *Service {
	return &Service{
		blockchainRepo: blockchainRepo,
		paymentRepo:    paymentRepo,
	}
}
