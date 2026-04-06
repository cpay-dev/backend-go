package workers

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/cpay-dev/cpay/internal/shared/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func parseFloat(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return v
}

func addInterval(t time.Time, unit string, count int) time.Time {
	if count <= 0 {
		count = 1
	}
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "day":
		return t.Add(time.Duration(count) * 24 * time.Hour)
	case "week":
		return t.Add(time.Duration(count*7) * 24 * time.Hour)
	default:
		return t.AddDate(0, count, 0)
	}
}

func enqueueOutboxTx(ctx context.Context, tx pgx.Tx, source, aggregateType, aggregateID, merchantID, eventType string, payload any) error {
	env, err := events.NewEnvelope(eventType, source, merchantID, aggregateID, payload)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO platform.outbox_events(id, aggregate_type, aggregate_id, merchant_id, event_type, payload, headers, status, available_at, created_at, updated_at)
		VALUES($1, $2, $3, $4, $5, $6::jsonb, '{}'::jsonb, 'pending', NOW(), NOW(), NOW())
	`, uuid.New(), aggregateType, aggregateID, merchantID, eventType, string(raw))
	return err
}
