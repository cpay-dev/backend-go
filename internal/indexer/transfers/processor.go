package transfers

import (
	"context"

	"github.com/cpay-dev/proto-go/blockchain/v1/indexer"
)

type BlockProcessor interface {
	Process(ctx context.Context, block *indexer.ParsedBlock) error
}
