package chain

import (
	pgblockchain "github.com/cpay-dev/backend-go/internal/api/repo/pg/blockchain"
	pbchain "github.com/cpay-dev/proto-go/api/v1/merchant/chain"
)

var _ pbchain.ChainServiceServer = (*Service)(nil)

type Service struct {
	pbchain.UnsafeChainServiceServer
	blockchainRepo *pgblockchain.PostgresRepo
}

func NewService(blockchainRepo *pgblockchain.PostgresRepo) *Service {
	return &Service{blockchainRepo: blockchainRepo}
}
