package blockchain

import (
	"context"
	"encoding/json"

	"github.com/cpay-dev/backend-go/internal/api/repo/pg/blockchain/model"
	"github.com/jackc/pgx/v5"
)

type Asset struct {
	ID       string          `db:"id"`
	ChainID  model.Chain     `db:"chain_id"`
	Name     string          `db:"name"`
	Symbol   string          `db:"symbol"`
	Metadata json.RawMessage `db:"metadata"`
}

func (r *PostgresRepo) CreateAsset(ctx context.Context, asset Asset) error {
	conn := r.GetConnectionFromCtx(ctx)

	const query = `
		INSERT INTO blockchain.assets 
		(id, chain_id, name, symbol, metadata) 
		VALUES 
		(@id, @chain_id, @name, @symbol, @metadata);
	`

	_, err := conn.Exec(ctx, query, pgx.NamedArgs{
		"id":       asset.ID,
		"chain_id": asset.ChainID,
		"name":     asset.Name,
		"symbol":   asset.Symbol,
		"metadata": asset.Metadata,
	})
	return err
}

func (r *PostgresRepo) ListAssets(ctx context.Context, chain model.Chain) ([]Asset, error) {
	conn := r.GetConnectionFromCtx(ctx)

	const queryAll = `
		SELECT 
			id, chain_id, name, symbol, metadata 
		FROM blockchain.assets 
		ORDER BY name;
	`

	const queryByChain = `
		SELECT 
			id, chain_id, name, symbol, metadata 
		FROM blockchain.assets 
		WHERE chain_id = $1 
		ORDER BY name;
	`

	var rows pgx.Rows

	switch chain {
	case model.ChainAny:
		rows, _ = conn.Query(ctx, queryAll)
	default:
		rows, _ = conn.Query(ctx, queryByChain, chain)
	}

	return pgx.CollectRows(rows, pgx.RowToStructByNameLax[Asset])
}
