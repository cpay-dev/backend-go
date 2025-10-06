package pg

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype/zeronull"
)

type Transfer struct {
	ID              string        `db:"id"`
	Chain           Chain         `db:"chain"`
	Kind            TransferKind  `db:"kind"`
	BlockHash       string        `db:"block_hash"`
	BlockNumber     uint64        `db:"block_number"`
	TxHash          string        `db:"tx_hash"`
	Index           uint16        `db:"index"`
	Amount          string        `db:"amount"`
	FromAddress     string        `db:"from_address"`
	ToAddress       string        `db:"to_address"`
	ContractAddress zeronull.Text `db:"contract_address"`
	BlockTimestamp  time.Time     `db:"block_timestamp"`
}

func (r *PostgresRepo) DeleteTransfersByBlockHash(ctx context.Context, chain Chain, blockHash string) error {
	conn := r.GetConnectionFromCtx(ctx)

	const query = `
		DELETE FROM indexer.transfers
		WHERE chain = @chain AND block_hash = @block_hash;
	`

	_, err := conn.Exec(ctx, query, pgx.NamedArgs{
		"block_hash": blockHash,
	})
	return err
}

func (r *PostgresRepo) DeleteTransfersByChain(ctx context.Context, chain Chain, cutoff time.Duration) (int, error) {
	conn := r.GetConnectionFromCtx(ctx)

	const query = `
		DELETE FROM indexer.transfers
		WHERE
			chain = @chain
			AND NOW() - block_timestamp > @cutoff;
	`

	res, err := conn.Exec(ctx, query, pgx.NamedArgs{
		"chain":  chain,
		"cutoff": cutoff,
	})
	return int(res.RowsAffected()), err
}

func (r *PostgresRepo) CreateTransfers(ctx context.Context, transfers []Transfer) error {
	conn := r.GetConnectionFromCtx(ctx)

	_, err := conn.Exec(ctx, "CREATE TEMPORARY TABLE transfers_temp (LIKE indexer.transfers) ON COMMIT DROP;")
	if err != nil {
		return fmt.Errorf("create temp table: %w", err)
	}

	_, err = conn.Exec(ctx, "ALTER TABLE transfers_temp ALTER COLUMN id SET DEFAULT gen_ulid();")
	if err != nil {
		return fmt.Errorf("modify temp table: %w", err)
	}

	_, err = conn.CopyFrom(ctx,
		pgx.Identifier{"transfers_temp"},
		[]string{
			"chain", "kind", "block_hash", "block_number", "tx_hash", "index", "amount", "from_address", "to_address", "contract_address", "block_timestamp",
		},
		pgx.CopyFromSlice(len(transfers), func(i int) ([]any, error) {
			return []any{
				transfers[i].Chain, transfers[i].Kind, transfers[i].BlockHash, transfers[i].BlockNumber, transfers[i].TxHash, transfers[i].Index,
				transfers[i].Amount, transfers[i].FromAddress, transfers[i].ToAddress, transfers[i].ContractAddress, transfers[i].BlockTimestamp,
			}, nil
		}),
	)
	if err != nil {
		return fmt.Errorf("copy from: %w", err)
	}

	_, err = conn.Exec(ctx, `
		INSERT INTO indexer.transfers 
			SELECT 
				id,
				chain,
				block_hash,
				block_number,
				tx_hash,
				index,
				amount,
				from_address,
				to_address,
				contract_address,
				kind,
				block_timestamp
			FROM transfers_temp;`)
	if err != nil {
		return fmt.Errorf("insert transfers: %w", err)
	}

	return nil
}

func (r *PostgresRepo) CreateTransfer(ctx context.Context, transfer Transfer) error {
	conn := r.GetConnectionFromCtx(ctx)

	const query = `
		INSERT INTO indexer.transfers
		(id, chain, kind, block_hash, block_number, tx_hash, index, amount, from_address, to_address, contract_address, block_timestamp)
		VALUES
		(gen_ulid(), @chain, @kind, @block_hash, @block_number, @tx_hash, @index, @amount, @from_address, @to_address, @contract_address, @block_timestamp);
	`

	_, err := conn.Exec(ctx, query, pgx.NamedArgs{
		"id":               transfer.ID,
		"chain":            transfer.Chain,
		"kind":             transfer.Kind,
		"block_hash":       transfer.BlockHash,
		"block_number":     transfer.BlockNumber,
		"tx_hash":          transfer.TxHash,
		"index":            transfer.Index,
		"amount":           transfer.Amount,
		"from_address":     transfer.FromAddress,
		"to_address":       transfer.ToAddress,
		"contract_address": transfer.ContractAddress,
		"block_timestamp":  transfer.BlockTimestamp,
	})
	return err
}
