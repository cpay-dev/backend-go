package asset

import (
	"context"
	"errors"
	"fmt"

	apiasset "github.com/cpay-dev/backend-go/internal/api/asset"
	assetmodel "github.com/cpay-dev/backend-go/internal/api/grpc/merchant/asset/model"
	chainmodel "github.com/cpay-dev/backend-go/internal/api/grpc/merchant/chain/model"
	pbasset "github.com/cpay-dev/proto-go/api/v1/merchant/asset"
	pbblockchain "github.com/cpay-dev/proto-go/blockchain/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Service) ListAssets(ctx context.Context, req *pbasset.ListAssetsRequest) (*pbasset.ListAssetsResponse, error) {
	switch req.ChainId {
	case pbblockchain.Chain_CHAIN_ANY_BTC, pbblockchain.Chain_CHAIN_ANY_EVM, pbblockchain.Chain_CHAIN_ANY_SVM:
		return nil, status.Error(codes.NotFound, "chain is not supported")
	}

	chain, err := chainmodel.ChainToRepo(req.ChainId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	assets, err := s.blockchainRepo.ListAssets(ctx, chain)
	if err != nil {
		return nil, fmt.Errorf("list assets: %w", err)
	}

	pbasets := make([]*pbasset.Asset, 0, len(assets))
	for _, asset := range assets {
		pbasset, err := assetmodel.AssetToProto(asset)
		if err != nil {
			return nil, fmt.Errorf("map asset to proto: %w", err)
		}

		pbasets = append(pbasets, pbasset)
	}

	return &pbasset.ListAssetsResponse{Assets: pbasets}, nil
}

func (s *Service) GetAssetPrice(ctx context.Context, req *pbasset.GetAssetPriceRequest) (*pbasset.GetAssetPriceResponse, error) {
	price, err := s.priceService.GetPrice(ctx, req.AssetId)
	if errors.Is(err, apiasset.ErrAssetNotFound) {
		return nil, status.Error(codes.NotFound, "asset not found")
	} else if errors.Is(err, apiasset.ErrPriceUnknown) {
		return nil, status.Error(codes.Internal, "price unknown")
	} else if err != nil {
		return nil, fmt.Errorf("get price: %w", err)
	}
	return &pbasset.GetAssetPriceResponse{Price: price}, nil
}
