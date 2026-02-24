package consumer

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/rs/zerolog/log"

	"github.com/cpay-dev/backend/internal/db"
	"github.com/cpay-dev/backend/internal/events"
)

// webhookEventMapping maps NATS event types to webhook event type names.
var webhookEventMapping = map[string]string{
	events.EventProductCreated:     "product.created",
	events.EventPaymentRecorded:    "payment.received",
	events.EventSubscriptionCreated: "subscription.created",
	events.EventSubscriptionPayment: "subscription.payment",
	events.EventWithdrawalCompleted: "withdrawal.completed",
	events.EventInvoiceCreated:      "invoice.created",
	events.EventInvoiceSent:         "invoice.sent",
	events.EventInvoicePaid:         "invoice.paid",
	events.EventInvoiceCancelled:    "invoice.cancelled",
}

type webhookPayload struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	CreatedAt string          `json:"created_at"`
	Data      json.RawMessage `json:"data"`
}

var webhookHTTPClient = &http.Client{
	Timeout: 10 * time.Second,
}

func (c *Consumer) startWebhookConsumers(ctx context.Context) error {
	if err := c.startConsumer(ctx, "PRODUCTS", "webhook-product-created",
		"products.product.created", c.handleWebhookDispatch); err != nil {
		return err
	}

	if err := c.startConsumer(ctx, "PAYMENTS", "webhook-payment-recorded",
		"payments.payment.recorded", c.handleWebhookDispatch); err != nil {
		return err
	}

	if err := c.startConsumer(ctx, "SUBSCRIPTIONS", "webhook-subscription-created",
		"subscriptions.subscription.created", c.handleWebhookDispatch); err != nil {
		return err
	}

	if err := c.startConsumer(ctx, "SUBSCRIPTIONS", "webhook-subscription-payment",
		"subscriptions.subscription.payment", c.handleWebhookDispatch); err != nil {
		return err
	}

	if err := c.startConsumer(ctx, "WITHDRAWALS", "webhook-withdrawal-completed",
		"withdrawals.withdrawal.completed", c.handleWebhookDispatch); err != nil {
		return err
	}

	if err := c.startConsumer(ctx, "INVOICES", "webhook-invoice-created",
		"invoices.invoice.created", c.handleWebhookDispatch); err != nil {
		return err
	}

	if err := c.startConsumer(ctx, "INVOICES", "webhook-invoice-sent",
		"invoices.invoice.sent", c.handleWebhookDispatch); err != nil {
		return err
	}

	if err := c.startConsumer(ctx, "INVOICES", "webhook-invoice-paid",
		"invoices.invoice.paid", c.handleWebhookDispatch); err != nil {
		return err
	}

	if err := c.startConsumer(ctx, "INVOICES", "webhook-invoice-cancelled",
		"invoices.invoice.cancelled", c.handleWebhookDispatch); err != nil {
		return err
	}

	return nil
}

func (c *Consumer) handleWebhookDispatch(ctx context.Context, msg jetstream.Msg) {
	evt, err := c.parseEvent(msg)
	if err != nil {
		log.Error().Err(err).Msg("webhook-dispatch: parse event")
		_ = msg.Ack()
		return
	}

	webhookEventType, ok := webhookEventMapping[evt.Type]
	if !ok {
		_ = msg.Ack()
		return
	}

	var data map[string]interface{}
	if err := json.Unmarshal(evt.Data, &data); err != nil {
		log.Error().Err(err).Msg("webhook-dispatch: unmarshal data")
		_ = msg.Ack()
		return
	}

	shopID, _ := data["shop_id"].(string)
	if shopID == "" {
		log.Warn().Str("event_id", evt.ID).Str("type", evt.Type).Msg("webhook-dispatch: missing shop_id")
		_ = msg.Ack()
		return
	}

	webhooks, err := c.queries.ListActiveWebhooksByShopAndEvent(ctx, db.ListActiveWebhooksByShopAndEventParams{
		ShopID:    shopID,
		EventType: webhookEventType,
	})
	if err != nil {
		log.Error().Err(err).Str("shop_id", shopID).Msg("webhook-dispatch: query webhooks")
		_ = msg.NakWithDelay(5 * time.Second)
		return
	}

	if len(webhooks) == 0 {
		_ = msg.Ack()
		return
	}

	payloadBytes, err := json.Marshal(webhookPayload{
		ID:        evt.ID,
		Type:      webhookEventType,
		CreatedAt: evt.Timestamp.Format(time.RFC3339),
		Data:      evt.Data,
	})
	if err != nil {
		log.Error().Err(err).Msg("webhook-dispatch: marshal payload")
		_ = msg.Ack()
		return
	}

	allSucceeded := true
	for _, wh := range webhooks {
		if err := c.deliverWebhook(ctx, wh, evt.ID, webhookEventType, payloadBytes); err != nil {
			log.Error().Err(err).Str("webhook_id", wh.ID).Str("url", wh.Url).Msg("webhook delivery failed")
			allSucceeded = false
		}
	}

	if allSucceeded {
		_ = msg.Ack()
	} else {
		_ = msg.NakWithDelay(30 * time.Second)
	}
}

func (c *Consumer) deliverWebhook(ctx context.Context, wh db.Webhook, eventID, eventType string, payloadBytes []byte) error {
	// Idempotency: check for existing delivery
	existing, err := c.queries.GetWebhookDeliveryByEventAndWebhook(ctx, db.GetWebhookDeliveryByEventAndWebhookParams{
		EventID:   eventID,
		WebhookID: wh.ID,
	})
	if err == nil && existing.Status == db.WebhookDeliveryStatusSuccess {
		return nil // already delivered
	}

	var deliveryID string
	var attempts int32
	if err == nil {
		deliveryID = existing.ID
		attempts = existing.Attempts
	} else {
		delivery, createErr := c.queries.CreateWebhookDelivery(ctx, db.CreateWebhookDeliveryParams{
			WebhookID: wh.ID,
			EventID:   eventID,
			EventType: eventType,
			Payload:   payloadBytes,
		})
		if createErr != nil {
			return fmt.Errorf("create delivery record: %w", createErr)
		}
		deliveryID = delivery.ID
	}

	// Compute HMAC-SHA256 signature
	mac := hmac.New(sha256.New, []byte(wh.Secret))
	mac.Write(payloadBytes)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, wh.Url, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CPay-Signature", signature)
	req.Header.Set("X-CPay-Event", eventType)

	resp, err := webhookHTTPClient.Do(req)
	attempts++

	if err != nil {
		errMsg := err.Error()
		_ = c.queries.UpdateWebhookDelivery(ctx, db.UpdateWebhookDeliveryParams{
			ID:             deliveryID,
			Status:         db.WebhookDeliveryStatusFailed,
			Attempts:       attempts,
			LastStatusCode: nil,
			LastError:      &errMsg,
		})
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	statusCode := int32(resp.StatusCode)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_ = c.queries.UpdateWebhookDelivery(ctx, db.UpdateWebhookDeliveryParams{
			ID:             deliveryID,
			Status:         db.WebhookDeliveryStatusSuccess,
			Attempts:       attempts,
			LastStatusCode: &statusCode,
			LastError:      nil,
		})
		return nil
	}

	errMsg := fmt.Sprintf("HTTP %d", resp.StatusCode)
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		// Client error — permanent failure, no retry
		_ = c.queries.UpdateWebhookDelivery(ctx, db.UpdateWebhookDeliveryParams{
			ID:             deliveryID,
			Status:         db.WebhookDeliveryStatusFailed,
			Attempts:       attempts,
			LastStatusCode: &statusCode,
			LastError:      &errMsg,
		})
		return nil
	}

	// 5xx — record failure and return error to trigger retry
	_ = c.queries.UpdateWebhookDelivery(ctx, db.UpdateWebhookDeliveryParams{
		ID:             deliveryID,
		Status:         db.WebhookDeliveryStatusFailed,
		Attempts:       attempts,
		LastStatusCode: &statusCode,
		LastError:      &errMsg,
	})
	return fmt.Errorf("server error: HTTP %d", resp.StatusCode)
}
