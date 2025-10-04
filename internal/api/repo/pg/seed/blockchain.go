package seed

import (
	"context"
	"fmt"

	"github.com/goccy/go-json"

	merchantmodel "github.com/cpay-dev/backend-go/internal/api/grpc/merchant/asset/model"
	"github.com/cpay-dev/backend-go/internal/api/repo/pg/blockchain"
	pgmdodel "github.com/cpay-dev/backend-go/internal/api/repo/pg/blockchain/model"
	pbasset "github.com/cpay-dev/proto-go/api/v1/merchant/asset"
)

type BlockchainSeeder struct {
	repo *blockchain.PostgresRepo
}

func NewBlockchainSeeder(repo *blockchain.PostgresRepo) *BlockchainSeeder {
	return &BlockchainSeeder{repo: repo}
}

func (s *BlockchainSeeder) Seed(ctx context.Context) error {
	return s.repo.RunInTx(ctx, func(ctx context.Context) error {
		return s.seed(ctx)
	})
}

func (s *BlockchainSeeder) seed(ctx context.Context) error {
	chains := []blockchain.Chain{
		{ID: pgmdodel.ChainAny, Name: "Any chain"},
		{ID: pgmdodel.ChainAnyBitcoin, Name: "Any Bitcoin chain"},
		{ID: pgmdodel.ChainAnyEVM, Name: "Any EVM chain"},
		{ID: pgmdodel.ChainAnySVM, Name: "Any SVM chain"},

		{ID: pgmdodel.ChainBitcoin, Name: "Bitcoin"},
		{ID: pgmdodel.ChainEthereum, Name: "Ethereum"},
		{ID: pgmdodel.ChainPolygon, Name: "Polygon"},
		{ID: pgmdodel.ChainArbitrum, Name: "Arbitrum"},
		{ID: pgmdodel.ChainUnchain, Name: "Unichain"},
		{ID: pgmdodel.ChainSolana, Name: "Solana"},
	}

	for _, chain := range chains {
		if err := s.repo.CreateChain(ctx, chain); err != nil {
			return fmt.Errorf("create chain: %w", err)
		}
	}

	type asset struct {
		blockchain.Asset
		md *pbasset.AssetMetadata
	}

	assets := []asset{
		{
			Asset: blockchain.Asset{
				ID:      "01K40YW14CPAYUNICHAINUSDT0",
				ChainID: pgmdodel.ChainUnchain,
				Name:    "USDT0",
				Symbol:  "USDT0",
			},
			md: &pbasset.AssetMetadata{
				Address:  "0x9151434b16b9763660705744891fA906F660EcC5",
				Decimals: 6,
				IsStable: true,
			},
		},
	}

	for _, asset := range assets {
		var md json.RawMessage
		if asset.md != nil {
			var err error
			md, err = merchantmodel.MarshalMetadata(asset.md)
			if err != nil {
				return fmt.Errorf("marshal metadata of asset %s: %w", asset.ID, err)
			}
		}
		asset.Asset.Metadata = md

		if err := s.repo.CreateAsset(ctx, asset.Asset); err != nil {
			return fmt.Errorf("create asset: %w", err)
		}
	}

	return nil
}
