package blockchain

import (
	"context"

	"github.com/cpay-dev/backend-go/internal/api/repo/pg/blockchain/model"
	"github.com/jackc/pgx/v5"
)

type Chain struct {
	ID   model.Chain `db:"id"`
	Name string      `db:"name"`
}

func (r *PostgresRepo) CreateChain(ctx context.Context, chain Chain) error {
	conn := r.GetConnectionFromCtx(ctx)

	const query = `
		INSERT INTO blockchain.chains (id, name) VALUES (@id, @name);
	`

	_, err := conn.Exec(ctx, query, pgx.NamedArgs{
		"id":   chain.ID,
		"name": chain.Name,
	})
	return err
}

func (r *PostgresRepo) ListChains(ctx context.Context) ([]Chain, error) {
	conn := r.GetConnectionFromCtx(ctx)

	const query = `
		SELECT id, name FROM blockchain.chains ORDER BY name;
	`

	rows, _ := conn.Query(ctx, query)

	return pgx.CollectRows(rows, pgx.RowToStructByNameLax[Chain])
}
