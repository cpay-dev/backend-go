package transfers

import (
	"context"
	"fmt"
	"time"

	"github.com/cpay-dev/backend-go/internal/indexer/repo/pg"
	pbblockchain "github.com/cpay-dev/proto-go/blockchain/v1"
	"github.com/cpay-dev/proto-go/blockchain/v1/indexer"
	"github.com/jackc/pgx/v5/pgtype/zeronull"
	"github.com/rs/zerolog"
)

type unichainProcessor struct {
	repo   *pg.PostgresRepo
	logger zerolog.Logger
}

func NewUnichainProcessor(repo *pg.PostgresRepo, logger zerolog.Logger) BlockProcessor {
	return &unichainProcessor{repo: repo, logger: logger}
}

func (p *unichainProcessor) Process(ctx context.Context, block *indexer.ParsedBlock) error {
	return p.repo.RunInTx(ctx, func(ctx context.Context) error {
		return p.process(ctx, block)
	})
}

func (p *unichainProcessor) process(ctx context.Context, parsedBlock *indexer.ParsedBlock) error {
	block := parsedBlock.Block

	if block.Chain != pbblockchain.Chain_CHAIN_EVM_UNICHAIN {
		return fmt.Errorf("wrong chain: %s", block.Chain)
	}

	if err := p.repo.DeleteTransfersByBlockHash(ctx, pg.ChainUnichain, block.BlockHash); err != nil {
		return fmt.Errorf("delete transfers by block hash: %w", err)
	}

	dbTransfers := make([]pg.Transfer, 0, len(parsedBlock.Transfers))

	for _, transfer := range parsedBlock.Transfers {
		p.logger.Debug().
			Str("tx_hash", transfer.TxHash).
			Uint64("index", transfer.Index).
			Str("from", transfer.From).
			Str("to", transfer.To).
			Str("amount", transfer.Amount).
			Bool("native", transfer.GetNative()).
			Str("contract", transfer.GetContract()).
			Msg("processing transfer")
		kind := pg.TransferKindNative
		if transfer.GetContract() != "" {
			kind = pg.TransferKindContract
		}

		dbTransfers = append(dbTransfers, pg.Transfer{
			Chain:           pg.ChainUnichain,
			Kind:            kind,
			BlockHash:       block.BlockHash,
			BlockNumber:     block.BlockNumber,
			TxHash:          transfer.TxHash,
			Index:           uint16(transfer.Index),
			Amount:          transfer.Amount,
			FromAddress:     transfer.From,
			ToAddress:       transfer.To,
			ContractAddress: zeronull.Text(transfer.GetContract()),
			BlockTimestamp:  time.Unix(int64(block.BlockTimestamp), 0),
		})
	}

	err := p.repo.CreateTransfers(ctx, dbTransfers)
	if err != nil {
		return fmt.Errorf("create transfer: %w", err)
	}

	return nil
}
