package app

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type MerchantStatus string

const (
	MerchantStatusActive   MerchantStatus = "ACTIVE"
	MerchantStatusInactive MerchantStatus = "INACTIVE"
	MerchantStatusBanned   MerchantStatus = "BANNED"
)

type Merchant struct {
	ID        string         `db:"id"`
	UserID    string         `db:"user_id"`
	Name      string         `db:"name"`
	Status    MerchantStatus `db:"status"`
	CreatedAt time.Time      `db:"created_at"`
	UpdatedAt time.Time      `db:"updated_at"`
	DeletedAt *time.Time     `db:"deleted_at"`
}

type MerchantAPIKey struct {
	ID         string     `db:"id"`
	UserID     string     `db:"user_id"`
	MerchantID string     `db:"merchant_id"`
	Name       string     `db:"name"`
	Key        string     `db:"key"`
	CreatedAt  time.Time  `db:"created_at"`
	UpdatedAt  time.Time  `db:"updated_at"`
	DeletedAt  *time.Time `db:"deleted_at"`
}

func (r *PostgresRepo) CreateMerchant(ctx context.Context, merchant Merchant) error {
	conn := r.GetConnectionFromCtx(ctx)

	const query = `
		INSERT INTO app.merchants (id, user_id, name, status, created_at, updated_at)
		VALUES (@id, @user_id, @name, @status, NOW(), NOW());
	`

	_, err := conn.Exec(ctx, query, pgx.NamedArgs{
		"id":      merchant.ID,
		"user_id": merchant.UserID,
		"name":    merchant.Name,
		"status":  merchant.Status,
	})
	return err
}

func (r *PostgresRepo) CreateMerchantAPIKey(ctx context.Context, apiKey MerchantAPIKey) error {
	conn := r.GetConnectionFromCtx(ctx)

	const query = `
		INSERT INTO app.merchant_api_keys (id, user_id, merchant_id, name, key, created_at, updated_at)
		VALUES (@id, @user_id, @merchant_id, @name, @key, NOW(), NOW());
	`

	_, err := conn.Exec(ctx, query, pgx.NamedArgs{
		"id":          apiKey.ID,
		"user_id":     apiKey.UserID,
		"merchant_id": apiKey.MerchantID,
		"name":        apiKey.Name,
		"key":         apiKey.Key,
	})
	return err
}

func (r *PostgresRepo) GetMerchantByAPIKey(ctx context.Context, key string) (*Merchant, error) {
	conn := r.GetConnectionFromCtx(ctx)

	const query = `
		SELECT m.id, m.user_id, m.name, m.status, m.created_at, m.updated_at, m.deleted_at
		FROM app.merchant_api_keys k
		JOIN app.merchants m ON m.id = k.merchant_id
		WHERE
			k.key = $1
			AND k.deleted_at IS NULL
			AND m.deleted_at IS NULL;
	`

	rows, _ := conn.Query(ctx, query, key)
	merchant, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByNameLax[Merchant])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}

	return merchant, err
}
