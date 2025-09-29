package merchant

import (
	"context"
	"fmt"

	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant/model"
	pbapimerchant "github.com/cpay-dev/proto-go/api/v1/merchant"
	pbmerchant "github.com/cpay-dev/proto-go/api/v1/merchant"
	pbblockchain "github.com/cpay-dev/proto-go/blockchain/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Service) ListAssets(ctx context.Context, req *pbmerchant.ListAssetsRequest) (*pbmerchant.ListAssetsResponse, error) {
	switch req.ChainId {
	case pbblockchain.Chain_CHAIN_ANY_BTC, pbblockchain.Chain_CHAIN_ANY_EVM, pbblockchain.Chain_CHAIN_ANY_SVM:
		return nil, status.Error(codes.NotFound, "chain is not supported")
	}

	chain, err := model.ChainToRepo(req.ChainId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	assets, err := s.blockchainRepo.ListAssets(ctx, chain)
	if err != nil {
		return nil, fmt.Errorf("list assets: %w", err)
	}

	pbasets := make([]*pbapimerchant.Asset, 0, len(assets))
	for _, asset := range assets {
		pbasset, err := model.AssetToProto(asset)
		if err != nil {
			return nil, fmt.Errorf("map asset to proto: %w", err)
		}

		pbasets = append(pbasets, pbasset)
	}

	return &pbmerchant.ListAssetsResponse{Assets: pbasets}, nil
}

func (s *Service) GetAssetPrice(ctx context.Context, req *pbmerchant.GetAssetPriceRequest) (*pbmerchant.GetAssetPriceResponse, error) {
	asset, err := s.blockchainRepo.GetAsset(ctx, req.AssetId)
	if err != nil {
		return nil, fmt.Errorf("get asset: %w", err)
	}
	if asset == nil {
		return nil, status.Error(codes.NotFound, "asset not found")
	}
	metadata, err := model.UnmarshalMetadata(asset.Metadata)
	if err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}
	if metadata.IsStable {
		return &pbmerchant.GetAssetPriceResponse{Price: "1"}, nil
	}
	return nil, status.Error(codes.Unimplemented, "only stablecoins are supported")
}
