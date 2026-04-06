package workers

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
)

type OutboxRelay struct {
	DB           *pgxpool.Pool
	NATS         *nats.Conn
	Log          zerolog.Logger
	PollInterval time.Duration
}

func (w *OutboxRelay) Run(ctx context.Context) {
	if w.PollInterval <= 0 {
		w.PollInterval = 2 * time.Second
	}
	ticker := time.NewTicker(w.PollInterval)
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

func (w *OutboxRelay) process(ctx context.Context) {
	if w.NATS == nil {
		w.Log.Warn().Msg("outbox relay skipped: nats is not configured")
		return
	}
	tx, err := w.DB.Begin(ctx)
	if err != nil {
		w.Log.Error().Err(err).Msg("outbox: begin tx failed")
		return
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT id::text, event_type, payload::text, retry_count
		FROM platform.outbox_events
		WHERE status IN ('pending', 'failed') AND available_at <= NOW()
		ORDER BY created_at
		LIMIT 100
		FOR UPDATE SKIP LOCKED
	`)
	if err != nil {
		w.Log.Error().Err(err).Msg("outbox: query failed")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id, eventType, payload string
		var retry int
		if err := rows.Scan(&id, &eventType, &payload, &retry); err != nil {
			w.Log.Error().Err(err).Msg("outbox: scan failed")
			continue
		}
		subject := "events." + strings.TrimSpace(eventType)
		if err := w.NATS.Publish(subject, []byte(payload)); err != nil {
			backoff := time.Duration(retry+1) * time.Second * 2
			_, _ = tx.Exec(ctx, `
				UPDATE platform.outbox_events
				SET status='failed', retry_count=retry_count+1, last_error=$2, available_at=NOW()+$3::interval, updated_at=NOW()
				WHERE id=$1
			`, id, err.Error(), backoff.String())
			continue
		}
		_, _ = tx.Exec(ctx, `
			UPDATE platform.outbox_events
			SET status='published', published_at=NOW(), updated_at=NOW(), last_error=NULL
			WHERE id=$1
		`, id)
	}

	if err := tx.Commit(ctx); err != nil {
		w.Log.Error().Err(err).Msg("outbox: commit failed")
	}
}
