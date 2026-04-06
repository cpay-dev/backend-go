package workers

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

type ChainObserver struct {
	DB       *pgxpool.Pool
	Log      zerolog.Logger
	Interval time.Duration
	Source   string
}

func (w *ChainObserver) Run(ctx context.Context) {
	if w.Interval <= 0 {
		w.Interval = 15 * time.Second
	}
	if w.Source == "" {
		w.Source = "chain-observer-service"
	}
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()

	w.expireOldIntents(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.expireOldIntents(ctx)
		}
	}
}

func (w *ChainObserver) expireOldIntents(ctx context.Context) {
	tx, err := w.DB.Begin(ctx)
	if err != nil {
		w.Log.Error().Err(err).Msg("chain observer: begin tx failed")
		return
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT id::text, merchant_id::text, checkout_session_id::text
		FROM checkout.payment_intents
		WHERE status IN ('created', 'awaiting_funds', 'partial') AND expires_at < NOW()
		FOR UPDATE SKIP LOCKED
	`)
	if err != nil {
		w.Log.Error().Err(err).Msg("chain observer: query failed")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var intentID, merchantID, sessionID string
		if err := rows.Scan(&intentID, &merchantID, &sessionID); err != nil {
			continue
		}
		_, _ = tx.Exec(ctx, `UPDATE checkout.payment_intents SET status='expired', updated_at=NOW() WHERE id=$1`, intentID)
		_, _ = tx.Exec(ctx, `UPDATE checkout.checkout_sessions SET status='expired', updated_at=NOW() WHERE id=$1`, sessionID)
		_ = enqueueOutboxTx(ctx, tx, w.Source, "payment_intent", intentID, merchantID, "payment.expired", map[string]any{"payment_intent_id": intentID})
	}

	if err := tx.Commit(ctx); err != nil {
		w.Log.Error().Err(err).Msg("chain observer: commit failed")
	}
}
