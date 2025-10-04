package asset

import (
	"context"
	"fmt"

	assetmodel "github.com/cpay-dev/backend-go/internal/api/grpc/merchant/asset/model"
	pgblockchain "github.com/cpay-dev/backend-go/internal/api/repo/pg/blockchain"
)

type PriceService struct {
	blockchainRepo *pgblockchain.PostgresRepo
}

func NewPriceService(blockchainRepo *pgblockchain.PostgresRepo) *PriceService {
	return &PriceService{blockchainRepo: blockchainRepo}
}

func (s *PriceService) GetPrice(ctx context.Context, assetID string) (string, error) {
	asset, err := s.blockchainRepo.GetAsset(ctx, assetID)
	if err != nil {
		return "", fmt.Errorf("get asset: %w", err)
	}
	if asset == nil {
		return "", ErrAssetNotFound
	}
	metadata, err := assetmodel.UnmarshalMetadata(asset.Metadata)
	if err != nil {
		return "", fmt.Errorf("unmarshal metadata: %w", err)
	}
	if metadata.IsStable {
		return "1", nil
	}
	return "", ErrPriceUnknown
}
