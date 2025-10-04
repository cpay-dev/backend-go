package asset

import (
	apiasset "github.com/cpay-dev/backend-go/internal/api/asset"
	pgblockchain "github.com/cpay-dev/backend-go/internal/api/repo/pg/blockchain"
	pbasset "github.com/cpay-dev/proto-go/api/v1/merchant/asset"
)

var _ pbasset.AssetServiceServer = (*Service)(nil)

type Service struct {
	pbasset.UnsafeAssetServiceServer
	blockchainRepo *pgblockchain.PostgresRepo
	priceService   *apiasset.PriceService
}

func NewService(blockchainRepo *pgblockchain.PostgresRepo, priceService *apiasset.PriceService) *Service {
	return &Service{blockchainRepo: blockchainRepo, priceService: priceService}
}
