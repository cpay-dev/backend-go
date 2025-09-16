package merchant

import (
	"context"
	"fmt"

	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant/model"
	pbapiblockchain "github.com/cpay-dev/proto-go/api/v1/blockchain"
	pbblockchain "github.com/cpay-dev/proto-go/api/v1/blockchain"
	pbmerchant "github.com/cpay-dev/proto-go/api/v1/merchant"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Service) ListChains(ctx context.Context, req *pbmerchant.ListChainsRequest) (*pbmerchant.ListChainsResponse, error) {
	chains, err := s.repo.ListChains(ctx)
	if err != nil {
		return nil, err
	}

	pbchains := make([]*pbapiblockchain.Chain, 0, len(chains))
	for _, chain := range chains {
		pbchain, err := model.ChainToProto(chain.ID)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		pbchains = append(pbchains, &pbapiblockchain.Chain{
			Id:   pbchain,
			Name: chain.Name,
		})
	}

	return &pbmerchant.ListChainsResponse{Chains: pbchains}, nil
}

func (s *Service) ListAssets(ctx context.Context, req *pbmerchant.ListAssetsRequest) (*pbmerchant.ListAssetsResponse, error) {
	switch req.Chain {
	case pbblockchain.ChainID_CHAIN_ID_ANY_BTC, pbblockchain.ChainID_CHAIN_ID_ANY_EVM, pbblockchain.ChainID_CHAIN_ID_ANY_SVM:
		return nil, status.Error(codes.NotFound, "chain is not supported")
	}

	chainId, err := model.ChainIdToRepo(req.Chain)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	assets, err := s.repo.ListAssets(ctx, chainId)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	pbasets := make([]*pbapiblockchain.Asset, 0, len(assets))

	for _, asset := range assets {
		pbasset, err := model.AssetToProto(asset)
		if err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("map asset to proto: %s", err.Error()))
		}

		pbasets = append(pbasets, pbasset)
	}

	return &pbmerchant.ListAssetsResponse{Assets: pbasets}, nil
}
