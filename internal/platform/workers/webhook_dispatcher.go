package workers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	cryptox "github.com/cpay-dev/cpay/internal/shared/crypto"
	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
)

type WebhookDispatcher struct {
	DB                *pgxpool.Pool
	Log               zerolog.Logger
	HTTPTimeout       time.Duration
	DefaultMaxRetries int
	EncryptionKey     []byte
}

func (w *WebhookDispatcher) Run(ctx context.Context, nc *nats.Conn) {
	if w.HTTPTimeout <= 0 {
		w.HTTPTimeout = 8 * time.Second
	}
	if w.DefaultMaxRetries <= 0 {
		w.DefaultMaxRetries = 8
	}

	if nc != nil {
		_, err := nc.Subscribe("events.>", func(msg *nats.Msg) {
			if err := w.handleIncoming(msg.Data); err != nil {
				w.Log.Error().Err(err).Msg("webhook: handle event failed")
			}
		})
		if err != nil {
			w.Log.Error().Err(err).Msg("webhook: failed to subscribe to nats")
		}
	} else {
		w.Log.Warn().Msg("webhook dispatcher started without nats subscription")
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processScheduled(ctx)
		}
	}
}

type envelope struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	MerchantID string          `json:"merchant_id"`
	Data       json.RawMessage `json:"data"`
}

func (w *WebhookDispatcher) handleIncoming(payload []byte) error {
	var env envelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return err
	}
	if strings.TrimSpace(env.MerchantID) == "" {
		return nil
	}

	rows, err := w.DB.Query(context.Background(), `
		SELECT id::text, url, secret_encrypted, max_retries, events
		FROM checkout.webhook_endpoints
		WHERE merchant_id=$1 AND enabled=TRUE
	`, env.MerchantID)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var endpointID, url, secretEncrypted string
		var maxRetries int
		var eventsRaw []byte
		if err := rows.Scan(&endpointID, &url, &secretEncrypted, &maxRetries, &eventsRaw); err != nil {
			continue
		}
		if maxRetries <= 0 {
			maxRetries = w.DefaultMaxRetries
		}
		if !eventAllowed(eventsRaw, env.Type) {
			continue
		}
		deliveryID := ids.New()
		_, _ = w.DB.Exec(context.Background(), `
			INSERT INTO checkout.webhook_deliveries(
				id, webhook_endpoint_id, merchant_id, event_id, event_type, payload, attempt, status, created_at, updated_at
			)
			VALUES($1, $2, $3, $4, $5, $6::jsonb, 1, 'pending', NOW(), NOW())
		`, deliveryID, endpointID, env.MerchantID, env.ID, env.Type, string(payload))
		w.deliverAndUpdate(context.Background(), deliveryID, url, secretEncrypted, payload, env.ID, env.Type, 1, maxRetries)
	}
	return nil
}

func (w *WebhookDispatcher) processScheduled(ctx context.Context) {
	rows, err := w.DB.Query(ctx, `
		SELECT d.id::text, d.event_id, d.event_type, d.payload::text, d.attempt,
			e.url, e.secret_encrypted, e.max_retries
		FROM checkout.webhook_deliveries d
		JOIN checkout.webhook_endpoints e ON e.id=d.webhook_endpoint_id
		WHERE d.status='scheduled' AND d.next_retry_at <= NOW() AND e.enabled=TRUE
		LIMIT 100
	`)
	if err != nil {
		w.Log.Error().Err(err).Msg("webhook: scheduled query failed")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var deliveryID, eventID, eventType, payloadText, url, secretEncrypted string
		var attempt, maxRetries int
		if err := rows.Scan(&deliveryID, &eventID, &eventType, &payloadText, &attempt, &url, &secretEncrypted, &maxRetries); err != nil {
			continue
		}
		if maxRetries <= 0 {
			maxRetries = w.DefaultMaxRetries
		}
		w.deliverAndUpdate(ctx, deliveryID, url, secretEncrypted, []byte(payloadText), eventID, eventType, attempt, maxRetries)
	}
}

func (w *WebhookDispatcher) deliverAndUpdate(ctx context.Context, deliveryID, url, secretEncrypted string, payload []byte, eventID, eventType string, attempt, maxRetries int) {
	secret, err := cryptox.DecryptString(w.EncryptionKey, secretEncrypted)
	if err != nil {
		w.Log.Error().Err(err).Str("delivery_id", deliveryID).Msg("webhook: secret decrypt failed")
		_, _ = w.DB.Exec(ctx, `UPDATE checkout.webhook_deliveries SET status='failed', response_body=$2, updated_at=NOW() WHERE id=$1`, deliveryID, "secret decrypt failed")
		return
	}

	timestamp := time.Now().Unix()
	signature := signWebhook(secret, timestamp, payload)

	reqCtx, cancel := context.WithTimeout(ctx, w.HTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		w.scheduleFailure(ctx, deliveryID, attempt, maxRetries, 0, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CPay-Event-ID", eventID)
	req.Header.Set("X-CPay-Event-Type", eventType)
	req.Header.Set("X-CPay-Signature", signature)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		w.scheduleFailure(ctx, deliveryID, attempt, maxRetries, 0, err.Error())
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_, _ = w.DB.Exec(ctx, `
			UPDATE checkout.webhook_deliveries
			SET status='delivered', response_code=$2, response_body=$3, delivered_at=NOW(), updated_at=NOW()
			WHERE id=$1
		`, deliveryID, resp.StatusCode, string(body))
		return
	}
	w.scheduleFailure(ctx, deliveryID, attempt, maxRetries, resp.StatusCode, string(body))
}

func (w *WebhookDispatcher) scheduleFailure(ctx context.Context, deliveryID string, attempt, maxRetries, statusCode int, response string) {
	nextAttempt := attempt + 1
	if nextAttempt > maxRetries {
		_, _ = w.DB.Exec(ctx, `
			UPDATE checkout.webhook_deliveries
			SET status='failed', response_code=$2, response_body=$3, updated_at=NOW()
			WHERE id=$1
		`, deliveryID, statusCode, response)
		return
	}
	backoff := time.Duration(nextAttempt*nextAttempt) * time.Second
	_, _ = w.DB.Exec(ctx, `
		UPDATE checkout.webhook_deliveries
		SET status='scheduled', attempt=$2, response_code=$3, response_body=$4, next_retry_at=NOW()+$5::interval, updated_at=NOW()
		WHERE id=$1
	`, deliveryID, nextAttempt, statusCode, response, backoff.String())
}

func signWebhook(secret string, timestamp int64, payload []byte) string {
	msg := []byte(strconv.FormatInt(timestamp, 10) + "." + string(payload))
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(msg)
	sig := hex.EncodeToString(mac.Sum(nil))
	return "t=" + strconv.FormatInt(timestamp, 10) + ",v1=" + sig
}

func eventAllowed(raw []byte, eventType string) bool {
	if len(raw) == 0 {
		return true
	}
	var events []string
	if err := json.Unmarshal(raw, &events); err != nil {
		return false
	}
	if len(events) == 0 {
		return true
	}
	for _, evt := range events {
		evt = strings.TrimSpace(evt)
		if evt == "*" || evt == eventType {
			return true
		}
	}
	return false
}
