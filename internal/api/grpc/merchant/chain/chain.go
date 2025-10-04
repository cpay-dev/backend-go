package chain

import (
	"context"
	"fmt"

	chainmodel "github.com/cpay-dev/backend-go/internal/api/grpc/merchant/chain/model"
	pbchain "github.com/cpay-dev/proto-go/api/v1/merchant/chain"
)

func (s *Service) ListChains(ctx context.Context, req *pbchain.ListChainsRequest) (*pbchain.ListChainsResponse, error) {
	chains, err := s.blockchainRepo.ListChains(ctx)
	if err != nil {
		return nil, err
	}

	pbchains := make([]*pbchain.Chain, 0, len(chains))
	for _, chain := range chains {
		chainId, err := chainmodel.ChainToProto(chain.ID)
		if err != nil {
			return nil, fmt.Errorf("map chain to proto: %w", err)
		}

		pbchains = append(pbchains, &pbchain.Chain{
			Id:   chainId,
			Name: chain.Name,
		})
	}

	return &pbchain.ListChainsResponse{Chains: pbchains}, nil
}
