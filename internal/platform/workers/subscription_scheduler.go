package workers

import (
	"context"
	"time"

	"github.com/cpay-dev/cpay/internal/domain/subscription"
	"github.com/cpay-dev/cpay/internal/shared/format"
	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

type SubscriptionScheduler struct {
	DB       *pgxpool.Pool
	Log      zerolog.Logger
	Interval time.Duration
	Source   string
}

func (w *SubscriptionScheduler) Run(ctx context.Context) {
	if w.Interval <= 0 {
		w.Interval = 30 * time.Second
	}
	if w.Source == "" {
		w.Source = "subscription-service"
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

func (w *SubscriptionScheduler) process(ctx context.Context) {
	tx, err := w.DB.Begin(ctx)
	if err != nil {
		w.Log.Error().Err(err).Msg("subscription: begin failed")
		return
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT c.id::text, c.subscription_id::text, c.cycle_index, c.due_at, c.amount::text, c.retry_count,
			s.merchant_id::text, s.interval_unit, s.interval_count,
			v.id::text, v.remaining_amount::text, v.status
		FROM checkout.subscription_cycles c
		JOIN checkout.subscriptions s ON s.id=c.subscription_id
		JOIN checkout.vault_authorizations v ON v.subscription_id=s.id
		WHERE c.status='due' AND c.due_at <= NOW() AND s.status='active'
		FOR UPDATE SKIP LOCKED
	`)
	if err != nil {
		w.Log.Error().Err(err).Msg("subscription: query failed")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var cycleID, subID, merchantID, intervalUnit, vaultID, remainingRaw, vaultStatus string
		var cycleIndex, intervalCount, retryCount int
		var dueAt time.Time
		var amountRaw string
		if err := rows.Scan(&cycleID, &subID, &cycleIndex, &dueAt, &amountRaw, &retryCount,
			&merchantID, &intervalUnit, &intervalCount,
			&vaultID, &remainingRaw, &vaultStatus); err != nil {
			continue
		}
		amount := format.Float64OrZero(amountRaw)
		remaining := format.Float64OrZero(remainingRaw)

		if vaultStatus == "active" && remaining >= amount {
			newRemaining := remaining - amount
			_, _ = tx.Exec(ctx, `UPDATE checkout.vault_authorizations SET remaining_amount=$2, updated_at=NOW() WHERE id=$1`, vaultID, newRemaining)
			_, _ = tx.Exec(ctx, `UPDATE checkout.subscription_cycles SET status='paid', updated_at=NOW() WHERE id=$1`, cycleID)
			nextDue := subscription.AddInterval(dueAt, intervalUnit, intervalCount)
			nextEnd := subscription.AddInterval(nextDue, intervalUnit, intervalCount)
			_, _ = tx.Exec(ctx, `UPDATE checkout.subscriptions SET next_billing_at=$2, updated_at=NOW() WHERE id=$1`, subID, nextDue)
			_, _ = tx.Exec(ctx, `
				INSERT INTO checkout.subscription_cycles(id, subscription_id, cycle_index, period_start, period_end, due_at, status, amount, retry_count, created_at, updated_at)
				VALUES($1, $2, $3, $4, $5, $4, 'due', $6, 0, NOW(), NOW())
				ON CONFLICT (subscription_id, cycle_index) DO NOTHING
			`, ids.New(), subID, cycleIndex+1, nextDue, nextEnd, amount)
			_ = enqueueOutboxTx(ctx, tx, w.Source, "subscription", subID, merchantID, "subscription.debit_succeeded", map[string]any{
				"subscription_id": subID,
				"cycle_id":        cycleID,
				"amount":          amount,
				"remaining":       newRemaining,
			})
			continue
		}

		newRetry := retryCount + 1
		_, _ = tx.Exec(ctx, `UPDATE checkout.subscription_cycles SET status='failed', retry_count=$2, last_error='insufficient vault balance', updated_at=NOW() WHERE id=$1`, cycleID, newRetry)
		if newRetry >= 3 {
			_, _ = tx.Exec(ctx, `UPDATE checkout.subscriptions SET status='past_due', updated_at=NOW() WHERE id=$1`, subID)
		}
		_ = enqueueOutboxTx(ctx, tx, w.Source, "subscription", subID, merchantID, "subscription.cycle_due", map[string]any{
			"subscription_id": subID,
			"cycle_id":        cycleID,
			"amount":          amount,
			"retry_count":     newRetry,
		})
	}

	if err := tx.Commit(ctx); err != nil {
		w.Log.Error().Err(err).Msg("subscription: commit failed")
	}
}
