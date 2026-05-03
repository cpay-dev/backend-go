package outbox

import (
	"context"
	"encoding/json"

	"github.com/cpay-dev/cpay/internal/shared/events"
	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Publisher struct {
	DB     *pgxpool.Pool
	Source string
}

func New(db *pgxpool.Pool, source string) *Publisher {
	return &Publisher{DB: db, Source: source}
}

func (p *Publisher) Enqueue(ctx context.Context, aggregateType, aggregateID string, merchantID *string, eventType string, payload any) error {
	env, err := events.NewEnvelope(eventType, p.Source, merchantIDString(merchantID), aggregateID, payload)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return err
	}
	_, err = p.DB.Exec(ctx, `
		INSERT INTO platform.outbox_events(id, aggregate_type, aggregate_id, merchant_id, event_type, payload, headers, status, available_at, created_at, updated_at)
		VALUES($1, $2, $3, $4, $5, $6::jsonb, '{}'::jsonb, 'pending', NOW(), NOW(), NOW())
	`, ids.New(), aggregateType, aggregateID, merchantID, eventType, string(raw))
	return err
}

func (p *Publisher) EnqueueTx(ctx context.Context, tx pgx.Tx, aggregateType, aggregateID string, merchantID *string, eventType string, payload any) error {
	env, err := events.NewEnvelope(eventType, p.Source, merchantIDString(merchantID), aggregateID, payload)
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
	`, ids.New(), aggregateType, aggregateID, merchantID, eventType, string(raw))
	return err
}

func merchantIDString(id *string) string {
	if id == nil {
		return ""
	}
	return *id
}
