package merchant

import (
	"context"
	"fmt"

	"github.com/cpay-dev/backend-go/internal/api/grpc/merchant/model"
	pbapimerchant "github.com/cpay-dev/proto-go/api/v1/merchant"
	pbmerchant "github.com/cpay-dev/proto-go/api/v1/merchant"
)

func (s *Service) ListChains(ctx context.Context, req *pbmerchant.ListChainsRequest) (*pbmerchant.ListChainsResponse, error) {
	chains, err := s.blockchainRepo.ListChains(ctx)
	if err != nil {
		return nil, err
	}

	pbchains := make([]*pbapimerchant.Chain, 0, len(chains))
	for _, chain := range chains {
		pbchain, err := model.ChainToProto(chain.ID)
		if err != nil {
			return nil, fmt.Errorf("map chain to proto: %w", err)
		}

		pbchains = append(pbchains, &pbapimerchant.Chain{
			Id:   pbchain,
			Name: chain.Name,
		})
	}

	return &pbmerchant.ListChainsResponse{Chains: pbchains}, nil
}
