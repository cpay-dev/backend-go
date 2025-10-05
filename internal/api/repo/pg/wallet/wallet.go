package wallet

import (
	"context"
	"errors"
	"time"

	blockchainmodel "github.com/cpay-dev/backend-go/internal/api/repo/pg/blockchain/model"
	"github.com/jackc/pgx/v5"
)

type WalletStatus string

const (
	WalletStatusAvailable WalletStatus = "AVAILABLE"
	WalletStatusInUse     WalletStatus = "IN_USE"
	WalletStatusCooldown  WalletStatus = "COOLDOWN"
)

type Wallet struct {
	ID                  string                `db:"id"`
	ChainID             blockchainmodel.Chain `db:"chain_id"`
	Status              WalletStatus          `db:"status"`
	KekVersion          uint32                `db:"kek_version"`
	PublicKey           string                `db:"public_key"`
	EncryptedPrivateKey []byte                `db:"encrypted_private_key"`
	CreatedAt           time.Time             `db:"created_at"`
	UpdatedAt           time.Time             `db:"updated_at"`
}

func (r *PostgresRepo) CreateWallet(ctx context.Context, wallet Wallet) error {
	conn := r.GetConnectionFromCtx(ctx)

	const query = `
		INSERT INTO wallet.wallets (id, chain_id, status, kek_version, public_key, encrypted_private_key, created_at, updated_at)
		VALUES (gen_ulid(), @chain_id, @status, @kek_version, @public_key, @encrypted_private_key, NOW(), NOW());
	`

	_, err := conn.Exec(ctx, query, pgx.NamedArgs{
		"chain_id":              wallet.ChainID,
		"status":                wallet.Status,
		"kek_version":           wallet.KekVersion,
		"public_key":            wallet.PublicKey,
		"encrypted_private_key": wallet.EncryptedPrivateKey,
	})
	return err
}

func (r *PostgresRepo) AcquireAvailableWallet(ctx context.Context, chainID blockchainmodel.Chain) (*Wallet, error) {
	conn := r.GetConnectionFromCtx(ctx)

	const query = `
		WITH wallet_id AS (
			SELECT id
			FROM wallet.wallets
			WHERE
				chain_id = @chain_id
				AND status = @status_available
			ORDER BY updated_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE wallet.wallets
		SET status = @status_in_use, updated_at = NOW()
		WHERE id = (SELECT id FROM wallet_id)
		RETURNING id, chain_id, status, kek_version, public_key, encrypted_private_key, created_at, updated_at;
	`

	rows, _ := conn.Query(ctx, query, pgx.NamedArgs{
		"chain_id":         chainID,
		"status_available": WalletStatusAvailable,
		"status_in_use":    WalletStatusInUse,
	})

	wallet, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByNameLax[Wallet])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}

	return wallet, err
}

func (r *PostgresRepo) CooldownWallet(ctx context.Context, walletID string) error {
	conn := r.GetConnectionFromCtx(ctx)

	const query = `
		UPDATE wallet.wallets
		SET status = @status_in_use, updated_at = NOW()
		WHERE
			id = @wallet_id
			AND status = @status_available;
	`

	res, err := conn.Exec(ctx, query, pgx.NamedArgs{
		"status_in_use":    WalletStatusInUse,
		"status_available": WalletStatusAvailable,
		"wallet_id":        walletID,
	})
	if err != nil {
		return err
	} else if res.RowsAffected() == 0 {
		return errors.New("wallet was not in use")
	}
	return nil
}

func (r *PostgresRepo) MakeWalletsAvailable(ctx context.Context, chainID blockchainmodel.Chain, cutoff time.Duration) (int, error) {
	conn := r.GetConnectionFromCtx(ctx)

	const query = `
		UPDATE wallet.wallets
		SET status = @status_available, updated_at = NOW()
		WHERE
			chain_id = @chain_id
			AND status = @status_cooldown
			AND NOW() - updated_at >= @cutoff;
	`

	res, err := conn.Exec(ctx, query, pgx.NamedArgs{
		"status_available": WalletStatusAvailable,
		"status_cooldown":  WalletStatusCooldown,
		"cutoff":           cutoff,
		"chain_id":         chainID,
	})
	return int(res.RowsAffected()), err
}
