package api

import (
	"context"
	"encoding/json"

	"github.com/cpay-dev/cpay/internal/shared/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Server) enqueueEvent(ctx context.Context, aggregateType, aggregateID string, merchantID uuid.UUID, eventType string, payload any) error {
	env, err := events.NewEnvelope(eventType, s.cfg.ServiceName, merchantID.String(), aggregateID, payload)
	if err != nil {
		return err
	}
	body, err := json.Marshal(env)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO outbox_events(id, aggregate_type, aggregate_id, merchant_id, event_type, payload, headers, status, available_at, created_at, updated_at)
		VALUES($1, $2, $3, $4, $5, $6::jsonb, '{}'::jsonb, 'pending', NOW(), NOW(), NOW())
	`, uuid.New(), aggregateType, aggregateID, merchantID, eventType, string(body))
	return err
}

func (s *Server) enqueueEventTx(ctx context.Context, tx pgx.Tx, aggregateType, aggregateID string, merchantID uuid.UUID, eventType string, payload any) error {
	env, err := events.NewEnvelope(eventType, s.cfg.ServiceName, merchantID.String(), aggregateID, payload)
	if err != nil {
		return err
	}
	body, err := json.Marshal(env)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_events(id, aggregate_type, aggregate_id, merchant_id, event_type, payload, headers, status, available_at, created_at, updated_at)
		VALUES($1, $2, $3, $4, $5, $6::jsonb, '{}'::jsonb, 'pending', NOW(), NOW(), NOW())
	`, uuid.New(), aggregateType, aggregateID, merchantID, eventType, string(body))
	return err
}
