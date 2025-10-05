package payment

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type (
	IntentStatus         string
	IntentWalletType     string
	IntentTransferStatus string
)

const (
	IntentStatusAwaitingPayment IntentStatus = "AWAITING_PAYMENT"
	IntentStatusPaid            IntentStatus = "PAID"
	IntentStatusExpired         IntentStatus = "EXPIRED"
	IntentStatusAMLCheckPending IntentStatus = "AML_CHECK_PENDING"
	IntentStatusAMLCheckFailed  IntentStatus = "AML_CHECK_FAILED"
	IntentStatusRefundPending   IntentStatus = "REFUND_PENDING"
	IntentStatusRefunded        IntentStatus = "REFUNDED"

	IntentWalletTypeCustodial    IntentWalletType = "CUSTODIAL"
	IntentWalletTypeNonCustodial IntentWalletType = "NON_CUSTODIAL"

	IntentTransferStatusPending  IntentTransferStatus = "PENDING"
	IntentTransferStatusDropped  IntentTransferStatus = "DROPPED"
	IntentTransferStatusComplete IntentTransferStatus = "COMPLETE"
)

type Intent struct {
	ID              string       `db:"id"`
	MerchantID      string       `db:"merchant_id"`
	AssetID         string       `db:"asset_id"`
	Status          IntentStatus `db:"status"`
	AmountUSD       string       `db:"amount_usd"`
	AmountAsset     string       `db:"amount_asset"`
	AmountPaidAsset string       `db:"amount_paid_asset"`
	CreatedAt       time.Time    `db:"created_at"`
	UpdatedAt       time.Time    `db:"updated_at"`
}

type IntentWallet struct {
	ID                 string           `db:"id"`
	IntentID           string           `db:"intent_id"`
	WalletID           string           `db:"wallet_id"`
	WalletType         IntentWalletType `db:"wallet_type"`
	WalletAssetAddress string           `db:"wallet_asset_address"`
}

type IntentTransfer struct {
	ID          string               `db:"id"`
	IntentID    string               `db:"intent_id"`
	Status      IntentTransferStatus `db:"status"`
	AmountAsset string               `db:"amount_asset"`
	CreatedAt   time.Time            `db:"created_at"`
	UpdatedAt   time.Time            `db:"updated_at"`
}

func (r *PostgresRepo) CreateIntent(ctx context.Context, intent Intent) error {
	conn := r.GetConnectionFromCtx(ctx)

	const query = `
		INSERT INTO payment.intents (id, merchant_id, asset_id, status, amount_usd, amount_asset, amount_paid_asset, created_at, updated_at)
		VALUES (gen_ulid(), @merchant_id, @asset_id, @status, @amount_usd, @amount_asset, @amount_paid_asset, NOW(), NOW());
	`

	_, err := conn.Exec(ctx, query, pgx.NamedArgs{
		"merchant_id":       intent.MerchantID,
		"asset_id":          intent.AssetID,
		"status":            intent.Status,
		"amount_usd":        intent.AmountUSD,
		"amount_asset":      intent.AmountAsset,
		"amount_paid_asset": intent.AmountPaidAsset,
	})
	return err
}

func (r *PostgresRepo) GetIntent(ctx context.Context, id string, merchantID string) (*Intent, error) {
	conn := r.GetConnectionFromCtx(ctx)

	const query = `
		SELECT 
			id, merchant_id, asset_id, status, amount_usd, amount_asset, amount_paid_asset, created_at, updated_at
		FROM payment.intents 
		WHERE id = $1 AND merchant_id = $2;
	`

	rows, _ := conn.Query(ctx, query, id, merchantID)
	intent, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByNameLax[Intent])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return intent, err
}

func (r *PostgresRepo) CreateIntentWallet(ctx context.Context, intentWallet IntentWallet) error {
	conn := r.GetConnectionFromCtx(ctx)

	const query = `
		INSERT INTO payment.intent_wallets (id, intent_id, wallet_id, wallet_type, asset_address)
		VALUES (gen_ulid(), @intent_id, @wallet_id, @wallet_type, @asset_address);
	`

	_, err := conn.Exec(ctx, query, pgx.NamedArgs{
		"intent_id":     intentWallet.IntentID,
		"wallet_id":     intentWallet.WalletID,
		"wallet_type":   intentWallet.WalletType,
		"asset_address": intentWallet.WalletAssetAddress,
	})
	return err
}
