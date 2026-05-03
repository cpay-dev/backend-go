package workers

import (
	"context"
	"strings"
	"time"

	cryptox "github.com/cpay-dev/cpay/internal/shared/crypto"
	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

type PayoutScheduler struct {
	DB         *pgxpool.Pool
	Log        zerolog.Logger
	Interval   time.Duration
	Source     string
	Executor   PayoutExecutor
	EncryptKey []byte
}

type payoutWork struct {
	ID          string
	MerchantID  string
	TotalRaw    string
	Chain       string
	TokenSymbol string
}

type payoutItemWork struct {
	PayoutItemID        string
	PaymentIntentID     string
	MerchantID          string
	Chain               string
	TokenSymbol         string
	TokenAddress        string
	AmountRaw           string
	DepositAddress      string
	EncryptedPrivateKey string
	SettlementAddress   string
}

func (w *PayoutScheduler) Run(ctx context.Context) {
	if w.Interval <= 0 {
		w.Interval = 30 * time.Second
	}
	if w.Source == "" {
		w.Source = "payout-service"
	}
	if w.Executor == nil {
		w.Executor = MockPayoutExecutor{}
	}
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()

	w.process(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.process(ctx)
		}
	}
}

func (w *PayoutScheduler) process(ctx context.Context) {
	w.schedulePayouts(ctx)
	w.completeDuePayouts(ctx)
}

func (w *PayoutScheduler) schedulePayouts(ctx context.Context) {
	rows, err := w.DB.Query(ctx, `
		SELECT merchant_id::text, chain, token_symbol
		FROM checkout.payment_intents
		WHERE status='confirmed' AND id NOT IN (SELECT payment_intent_id FROM checkout.payout_items)
		GROUP BY merchant_id, chain, token_symbol
	`)
	if err != nil {
		w.Log.Error().Err(err).Msg("payout: schedule query failed")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var merchantID, chainName, tokenSymbol string
		if err := rows.Scan(&merchantID, &chainName, &tokenSymbol); err != nil {
			continue
		}
		tx, err := w.DB.Begin(ctx)
		if err != nil {
			continue
		}
		payoutID := ids.New()
		_, err = tx.Exec(ctx, `
			INSERT INTO checkout.payouts(id, merchant_id, status, schedule_at, token_symbol, chain, created_at, updated_at)
			VALUES($1, $2, 'scheduled', NOW(), $3, $4, NOW(), NOW())
		`, payoutID, merchantID, tokenSymbol, chainName)
		if err != nil {
			tx.Rollback(ctx)
			continue
		}

		itemRows, err := tx.Query(ctx, `
			SELECT id::text, received_amount::text
			FROM checkout.payment_intents
			WHERE merchant_id=$1 AND chain=$2 AND token_symbol=$3
				AND status='confirmed' AND id NOT IN (SELECT payment_intent_id FROM checkout.payout_items)
			FOR UPDATE SKIP LOCKED
		`, merchantID, chainName, tokenSymbol)
		if err != nil {
			tx.Rollback(ctx)
			continue
		}
		total := 0.0
		for itemRows.Next() {
			var intentID, amountRaw string
			if err := itemRows.Scan(&intentID, &amountRaw); err != nil {
				continue
			}
			amount := parseFloat(amountRaw)
			total += amount
			_, _ = tx.Exec(ctx, `
				INSERT INTO checkout.payout_items(id, payout_id, payment_intent_id, amount, created_at)
				VALUES($1, $2, $3, $4, NOW())
			`, ids.New(), payoutID, intentID, amount)
		}
		itemRows.Close()
		_, _ = tx.Exec(ctx, `UPDATE checkout.payouts SET total_amount=$2, updated_at=NOW() WHERE id=$1`, payoutID, total)

		_ = tx.Commit(ctx)
	}
}

func (w *PayoutScheduler) completeDuePayouts(ctx context.Context) {
	tx, err := w.DB.Begin(ctx)
	if err != nil {
		w.Log.Error().Err(err).Msg("payout: begin failed")
		return
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT id::text, merchant_id::text, total_amount::text, chain, token_symbol
		FROM checkout.payouts
		WHERE status IN ('scheduled', 'processing') AND schedule_at <= NOW()
		FOR UPDATE SKIP LOCKED
	`)
	if err != nil {
		w.Log.Error().Err(err).Msg("payout: due query failed")
		return
	}
	defer rows.Close()

	payouts := make([]payoutWork, 0)
	for rows.Next() {
		var item payoutWork
		if err := rows.Scan(&item.ID, &item.MerchantID, &item.TotalRaw, &item.Chain, &item.TokenSymbol); err != nil {
			continue
		}
		payouts = append(payouts, item)
		_, _ = tx.Exec(ctx, `
			UPDATE checkout.payouts
			SET status='processing', attempt_count=attempt_count+1, last_error=NULL, updated_at=NOW()
			WHERE id=$1
		`, item.ID)
	}
	rows.Close()

	if err := tx.Commit(ctx); err != nil {
		w.Log.Error().Err(err).Msg("payout: commit failed")
		return
	}

	for _, payout := range payouts {
		w.processPayout(ctx, payout)
	}
}

func (w *PayoutScheduler) processPayout(ctx context.Context, payout payoutWork) {
	rows, err := w.DB.Query(ctx, `
		SELECT pi.id::text, pi.payment_intent_id::text, i.merchant_id::text, i.chain, i.token_symbol,
			COALESCE(i.token_address, ''), pi.amount::text, d.address, d.encrypted_private_key,
			COALESCE(m.settlement_address, '')
		FROM checkout.payout_items pi
		JOIN checkout.payment_intents i ON i.id=pi.payment_intent_id
		JOIN checkout.deposit_addresses d ON d.payment_intent_id=i.id
		JOIN auth.merchants m ON m.id=i.merchant_id
		WHERE pi.payout_id=$1
			AND pi.status IN ('pending', 'processing', 'failed')
			AND i.status='confirmed'
		ORDER BY pi.created_at ASC
	`, payout.ID)
	if err != nil {
		w.markPayoutRetry(ctx, payout.ID, "failed to load payout items")
		w.Log.Error().Err(err).Str("payout_id", payout.ID).Msg("payout: item query failed")
		return
	}
	defer rows.Close()

	hadFailure := false
	for rows.Next() {
		var item payoutItemWork
		if err := rows.Scan(
			&item.PayoutItemID,
			&item.PaymentIntentID,
			&item.MerchantID,
			&item.Chain,
			&item.TokenSymbol,
			&item.TokenAddress,
			&item.AmountRaw,
			&item.DepositAddress,
			&item.EncryptedPrivateKey,
			&item.SettlementAddress,
		); err != nil {
			hadFailure = true
			continue
		}

		mockMode := isMockPayoutExecutor(w.Executor)
		if !mockMode && strings.TrimSpace(item.SettlementAddress) == "" {
			hadFailure = true
			w.markItemFailed(ctx, item.PayoutItemID, "merchant settlement address is not configured")
			continue
		}
		privateKey := ""
		if !mockMode {
			var err error
			privateKey, err = cryptox.DecryptString(w.EncryptKey, item.EncryptedPrivateKey)
			if err != nil {
				hadFailure = true
				w.markItemFailed(ctx, item.PayoutItemID, "failed to decrypt deposit private key")
				continue
			}
		}

		_, _ = w.DB.Exec(ctx, `
			UPDATE checkout.payout_items
			SET status='processing', last_error=NULL
			WHERE id=$1 AND status IN ('pending', 'failed', 'processing')
		`, item.PayoutItemID)

		result, err := w.Executor.ExecutePayout(ctx, PayoutExecutionRequest{
			PayoutID:             payout.ID,
			PayoutItemID:         item.PayoutItemID,
			PaymentIntentID:      item.PaymentIntentID,
			MerchantID:           item.MerchantID,
			Chain:                item.Chain,
			TokenSymbol:          item.TokenSymbol,
			TokenAddress:         item.TokenAddress,
			AmountRaw:            item.AmountRaw,
			DepositAddress:       item.DepositAddress,
			DepositPrivateKeyHex: privateKey,
			SettlementAddress:    item.SettlementAddress,
		})
		if err != nil {
			hadFailure = true
			w.markItemFailed(ctx, item.PayoutItemID, err.Error())
			continue
		}
		if err := w.completePayoutItem(ctx, item, result.TxHash); err != nil {
			hadFailure = true
			w.markItemFailed(ctx, item.PayoutItemID, "failed to persist payout result")
			w.Log.Error().Err(err).Str("payout_item_id", item.PayoutItemID).Msg("payout: persist item failed")
		}
	}
	if rows.Err() != nil {
		hadFailure = true
	}
	rows.Close()

	if err := w.finishPayoutIfComplete(ctx, payout); err != nil {
		w.Log.Error().Err(err).Str("payout_id", payout.ID).Msg("payout: finish failed")
		w.markPayoutRetry(ctx, payout.ID, "failed to finish payout")
		return
	}
	if hadFailure {
		w.markPayoutRetry(ctx, payout.ID, "one or more payout items failed")
	}
}

func (w *PayoutScheduler) completePayoutItem(ctx context.Context, item payoutItemWork, txHash string) error {
	tx, err := w.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE checkout.payout_items
		SET status='completed', tx_hash=$2, last_error=NULL, completed_at=NOW()
		WHERE id=$1
	`, item.PayoutItemID, txHash); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE checkout.payment_intents
		SET status='settled', settled_at=NOW(), updated_at=NOW()
		WHERE id=$1 AND status='confirmed'
	`, item.PaymentIntentID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE checkout.deposit_addresses
		SET status='swept'
		WHERE payment_intent_id=$1
	`, item.PaymentIntentID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (w *PayoutScheduler) finishPayoutIfComplete(ctx context.Context, payout payoutWork) error {
	tx, err := w.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var remaining int64
	var txHash string
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE status!='completed'), COALESCE(MAX(tx_hash), '')
		FROM checkout.payout_items
		WHERE payout_id=$1
	`, payout.ID).Scan(&remaining, &txHash); err != nil {
		return err
	}
	if remaining > 0 {
		return tx.Commit(ctx)
	}
	cmd, err := tx.Exec(ctx, `
		UPDATE checkout.payouts
		SET status='completed', tx_hash=$2, last_error=NULL, completed_at=NOW(), updated_at=NOW()
		WHERE id=$1 AND status!='completed'
	`, payout.ID, txHash)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() > 0 {
		if err := enqueueOutboxTx(ctx, tx, w.Source, "payout", payout.ID, payout.MerchantID, "payout.completed", map[string]any{
			"payout_id":    payout.ID,
			"tx_hash":      txHash,
			"chain":        payout.Chain,
			"token_symbol": payout.TokenSymbol,
			"total_amount": parseFloat(payout.TotalRaw),
		}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (w *PayoutScheduler) markItemFailed(ctx context.Context, payoutItemID, message string) {
	_, _ = w.DB.Exec(ctx, `
		UPDATE checkout.payout_items
		SET status='failed', last_error=$2
		WHERE id=$1
	`, payoutItemID, truncateError(message))
}

func (w *PayoutScheduler) markPayoutRetry(ctx context.Context, payoutID, message string) {
	_, _ = w.DB.Exec(ctx, `
		UPDATE checkout.payouts
		SET status='scheduled', last_error=$2, updated_at=NOW()
		WHERE id=$1 AND status!='completed'
	`, payoutID, truncateError(message))
}

func truncateError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > 500 {
		return message[:500]
	}
	return message
}

func isMockPayoutExecutor(executor PayoutExecutor) bool {
	switch executor.(type) {
	case MockPayoutExecutor, *MockPayoutExecutor:
		return true
	default:
		return false
	}
}
