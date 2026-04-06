package workers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

type PayoutScheduler struct {
	DB       *pgxpool.Pool
	Log      zerolog.Logger
	Interval time.Duration
	Source   string
}

func (w *PayoutScheduler) Run(ctx context.Context) {
	if w.Interval <= 0 {
		w.Interval = 30 * time.Second
	}
	if w.Source == "" {
		w.Source = "payout-service"
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
		payoutID := uuid.New()
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
			`, uuid.New(), payoutID, intentID, amount)
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
		WHERE status='scheduled' AND schedule_at <= NOW()
		FOR UPDATE SKIP LOCKED
	`)
	if err != nil {
		w.Log.Error().Err(err).Msg("payout: due query failed")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var payoutID, merchantID, totalRaw, chainName, tokenSymbol string
		if err := rows.Scan(&payoutID, &merchantID, &totalRaw, &chainName, &tokenSymbol); err != nil {
			continue
		}
		txHash := fmt.Sprintf("0xsweep_%s", strings.ReplaceAll(uuid.NewString(), "-", ""))
		_, _ = tx.Exec(ctx, `
			UPDATE checkout.payouts
			SET status='completed', tx_hash=$2, completed_at=NOW(), updated_at=NOW()
			WHERE id=$1
		`, payoutID, txHash)
		_, _ = tx.Exec(ctx, `
			UPDATE checkout.payment_intents
			SET status='settled', settled_at=NOW(), updated_at=NOW()
			WHERE id IN (SELECT payment_intent_id FROM checkout.payout_items WHERE payout_id=$1) AND status='confirmed'
		`, payoutID)
		_ = enqueueOutboxTx(ctx, tx, w.Source, "payout", payoutID, merchantID, "payout.completed", map[string]any{
			"payout_id":    payoutID,
			"tx_hash":      txHash,
			"chain":        chainName,
			"token_symbol": tokenSymbol,
			"total_amount": parseFloat(totalRaw),
		})
	}

	if err := tx.Commit(ctx); err != nil {
		w.Log.Error().Err(err).Msg("payout: commit failed")
	}
}
