package consumer

import (
	"context"
	"encoding/json"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/cpay-dev/backend/internal/db"
	"github.com/cpay-dev/backend/internal/email"
	"github.com/cpay-dev/backend/internal/events"
	grpcclient "github.com/cpay-dev/backend/internal/grpc"
)

type Consumer struct {
	js      jetstream.JetStream
	queries *db.Queries
	email   *email.Service
	chain   *grpcclient.ChainClient
}

func New(js jetstream.JetStream, queries *db.Queries, emailSvc *email.Service, chain *grpcclient.ChainClient) *Consumer {
	return &Consumer{
		js:      js,
		queries: queries,
		email:   emailSvc,
		chain:   chain,
	}
}

func (c *Consumer) Start(ctx context.Context) error {
	// Payment verifier: verify tx hash on-chain
	if err := c.startConsumer(ctx, "SUBSCRIPTIONS", "payment-verifier",
		"subscriptions.subscription.payment", c.handlePaymentVerification); err != nil {
		return err
	}

	// Email-dependent consumers (skip if email service is nil)
	if c.email != nil {
		// Payment receipts (products, payment links)
		if err := c.startConsumer(ctx, "PAYMENTS", "payment-receipt-sender",
			"payments.payment.recorded", c.handlePaymentReceiptSend); err != nil {
			return err
		}

		// Subscription receipts
		if err := c.startConsumer(ctx, "SUBSCRIPTIONS", "receipt-sender",
			"subscriptions.subscription.payment", c.handleReceiptSend); err != nil {
			return err
		}
		if err := c.startConsumer(ctx, "SUBSCRIBERS", "subscriber-cancelled",
			"subscribers.subscriber.cancelled", c.handleSubscriberCancelled); err != nil {
			return err
		}
		if err := c.startConsumer(ctx, "SUBSCRIBERS", "subscriber-expired",
			"subscribers.subscriber.expired", c.handleSubscriberExpired); err != nil {
			return err
		}
		if err := c.startConsumer(ctx, "SUBSCRIBERS", "subscriber-failed",
			"subscribers.subscriber.failed", c.handleSubscriberFailed); err != nil {
			return err
		}

		// Generic email queue (scheduler reminders, future email types)
		if err := c.startConsumer(ctx, "EMAILS", "email-sender",
			"emails.email.queued", c.handleEmailQueued); err != nil {
			return err
		}
	} else {
		log.Warn().Msg("email service not configured, skipping email consumers")
	}

	// Webhook dispatcher consumers (always active, independent of email)
	if err := c.startWebhookConsumers(ctx); err != nil {
		return err
	}

	log.Info().Msg("all NATS consumers started")
	return nil
}

func (c *Consumer) startConsumer(ctx context.Context, stream, name, filter string, handler func(context.Context, jetstream.Msg)) error {
	cons, err := c.js.CreateOrUpdateConsumer(ctx, stream, jetstream.ConsumerConfig{
		Durable:       name,
		FilterSubject: filter,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    5,
		AckWait:       30 * time.Second,
	})
	if err != nil {
		return err
	}

	go func() {
		for {
			msgs, err := cons.Fetch(1, jetstream.FetchMaxWait(10*time.Second))
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				continue
			}
			for msg := range msgs.Messages() {
				handler(ctx, msg)
			}
		}
	}()

	log.Info().Str("stream", stream).Str("consumer", name).Str("filter", filter).Msg("consumer started")
	return nil
}

type paymentEventData struct {
	ID             string `json:"id"`
	SubscriptionID string `json:"subscription_id"`
	TxHash         string `json:"tx_hash"`
	ChainID        int64  `json:"chain_id"`
	PayerAddress   string `json:"payer_address"`
	ShopID         string `json:"shop_id"`
	Amount         string `json:"amount"`
}

func (c *Consumer) parseEvent(msg jetstream.Msg) (*events.Event, error) {
	var evt events.Event
	if err := json.Unmarshal(msg.Data(), &evt); err != nil {
		return nil, err
	}
	return &evt, nil
}

func (c *Consumer) handlePaymentVerification(ctx context.Context, msg jetstream.Msg) {
	evt, err := c.parseEvent(msg)
	if err != nil {
		log.Error().Err(err).Msg("payment-verifier: parse event")
		_ = msg.Nak()
		return
	}

	var data paymentEventData
	if err := json.Unmarshal(evt.Data, &data); err != nil {
		log.Error().Err(err).Msg("payment-verifier: unmarshal data")
		_ = msg.Nak()
		return
	}

	if data.TxHash == "" || data.ChainID == 0 {
		log.Warn().Str("payment_id", data.ID).Msg("payment-verifier: missing tx_hash or chain_id")
		_ = msg.Ack()
		return
	}

	success, _, _, err := c.chain.VerifyTransaction(ctx, uint64(data.ChainID), data.TxHash)
	if err != nil {
		log.Error().Err(err).Str("tx_hash", data.TxHash).Msg("payment-verifier: verify failed")
		_ = msg.Nak()
		return
	}

	if success {
		_ = c.queries.MarkPaymentVerified(ctx, data.ID)
		log.Info().Str("payment_id", data.ID).Str("tx_hash", data.TxHash).Msg("payment verified")
	} else {
		log.Warn().Str("payment_id", data.ID).Str("tx_hash", data.TxHash).Msg("payment tx failed on-chain")
	}

	_ = msg.Ack()
}

func (c *Consumer) handleReceiptSend(ctx context.Context, msg jetstream.Msg) {
	evt, err := c.parseEvent(msg)
	if err != nil {
		log.Error().Err(err).Msg("receipt-sender: parse event")
		_ = msg.Nak()
		return
	}

	var data paymentEventData
	if err := json.Unmarshal(evt.Data, &data); err != nil {
		log.Error().Err(err).Msg("receipt-sender: unmarshal data")
		_ = msg.Nak()
		return
	}

	// Get subscription title
	sub, err := c.queries.GetSubscription(ctx, data.SubscriptionID)
	if err != nil {
		log.Error().Err(err).Msg("receipt-sender: get subscription")
		_ = msg.Ack()
		return
	}

	// Find subscriber for payer email
	subscriber, err := c.queries.GetSubscriber(ctx, db.GetSubscriberParams{
		SubscriptionID: data.SubscriptionID,
		PayerAddress:   data.PayerAddress,
	})
	if err == nil && subscriber.PayerEmail != "" {
		var nextDue time.Time
		if subscriber.NextDue.Valid {
			nextDue = subscriber.NextDue.Time
		}
		if err := c.email.SendReceipt(subscriber.PayerEmail, sub.Title, data.Amount, data.TxHash, nextDue); err != nil {
			log.Error().Err(err).Str("payment_id", data.ID).Str("to", subscriber.PayerEmail).Msg("receipt-sender: send failed, will retry")
			_ = msg.NakWithDelay(10 * time.Second)
			return
		}
	}

	// Notify merchant
	merchantEmail, err := c.queries.GetMerchantEmail(ctx, data.ShopID)
	if err == nil && merchantEmail != nil && *merchantEmail != "" {
		if err := c.email.SendMerchantPaymentNotification(*merchantEmail, sub.Title, data.Amount, data.PayerAddress); err != nil {
			log.Error().Err(err).Str("payment_id", data.ID).Str("to", *merchantEmail).Msg("receipt-sender: merchant notify failed, will retry")
			_ = msg.NakWithDelay(10 * time.Second)
			return
		}
	}

	_ = msg.Ack()
}

func (c *Consumer) handleSubscriberCancelled(ctx context.Context, msg jetstream.Msg) {
	evt, err := c.parseEvent(msg)
	if err != nil {
		log.Error().Err(err).Msg("subscriber-cancelled: parse event")
		_ = msg.Nak()
		return
	}

	var data struct {
		ID             string `json:"id"`
		SubscriptionID string `json:"subscription_id"`
		Payer          string `json:"payer"`
		Email          string `json:"email"`
	}
	if err := json.Unmarshal(evt.Data, &data); err != nil {
		log.Error().Err(err).Msg("subscriber-cancelled: unmarshal data")
		_ = msg.Nak()
		return
	}

	sub, err := c.queries.GetSubscription(ctx, data.SubscriptionID)
	if err != nil {
		log.Error().Err(err).Str("subscription_id", data.SubscriptionID).Msg("subscriber-cancelled: get subscription")
		_ = msg.Ack()
		return
	}

	if data.Email != "" {
		if err := c.email.SendCancelled(data.Email, sub.Title); err != nil {
			log.Error().Err(err).Str("subscriber_id", data.ID).Str("to", data.Email).Msg("subscriber-cancelled: send failed, will retry")
			_ = msg.NakWithDelay(10 * time.Second)
			return
		}
	}

	_ = msg.Ack()
}

func (c *Consumer) handleSubscriberExpired(ctx context.Context, msg jetstream.Msg) {
	evt, err := c.parseEvent(msg)
	if err != nil {
		log.Error().Err(err).Msg("subscriber-expired: parse event")
		_ = msg.Nak()
		return
	}

	var data struct {
		ID             string `json:"id"`
		SubscriptionID string `json:"subscription_id"`
		Email          string `json:"email"`
	}
	if err := json.Unmarshal(evt.Data, &data); err != nil {
		log.Error().Err(err).Msg("subscriber-expired: unmarshal data")
		_ = msg.Nak()
		return
	}

	sub, err := c.queries.GetSubscription(ctx, data.SubscriptionID)
	if err != nil {
		log.Error().Err(err).Str("subscription_id", data.SubscriptionID).Msg("subscriber-expired: get subscription")
		_ = msg.Ack()
		return
	}

	if data.Email != "" {
		if err := c.email.SendExpired(data.Email, sub.Title); err != nil {
			log.Error().Err(err).Str("subscriber_id", data.ID).Str("to", data.Email).Msg("subscriber-expired: send failed, will retry")
			_ = msg.NakWithDelay(10 * time.Second)
			return
		}
	}

	_ = msg.Ack()
}

func (c *Consumer) handleSubscriberFailed(ctx context.Context, msg jetstream.Msg) {
	evt, err := c.parseEvent(msg)
	if err != nil {
		log.Error().Err(err).Msg("subscriber-failed: parse event")
		_ = msg.Nak()
		return
	}

	var data struct {
		ID             string `json:"id"`
		SubscriptionID string `json:"subscription_id"`
		Email          string `json:"email"`
		Amount         string `json:"amount"`
		Reason         string `json:"reason"`
	}
	if err := json.Unmarshal(evt.Data, &data); err != nil {
		log.Error().Err(err).Msg("subscriber-failed: unmarshal data")
		_ = msg.Nak()
		return
	}

	sub, err := c.queries.GetSubscription(ctx, data.SubscriptionID)
	if err != nil {
		log.Error().Err(err).Str("subscription_id", data.SubscriptionID).Msg("subscriber-failed: get subscription")
		_ = msg.Ack()
		return
	}

	if data.Email != "" {
		if err := c.email.SendPaymentFailed(data.Email, sub.Title, data.Amount, data.Reason); err != nil {
			log.Error().Err(err).Str("subscriber_id", data.ID).Str("to", data.Email).Msg("subscriber-failed: send failed, will retry")
			_ = msg.NakWithDelay(10 * time.Second)
			return
		}
	}

	_ = msg.Ack()
}

func (c *Consumer) handleEmailQueued(_ context.Context, msg jetstream.Msg) {
	evt, err := c.parseEvent(msg)
	if err != nil {
		log.Error().Err(err).Msg("email-sender: parse event")
		_ = msg.Nak()
		return
	}

	var data events.EmailQueuedData
	if err := json.Unmarshal(evt.Data, &data); err != nil {
		log.Error().Err(err).Msg("email-sender: unmarshal data")
		_ = msg.Nak()
		return
	}

	if data.To == "" {
		log.Warn().Str("email_type", data.EmailType).Msg("email-sender: empty recipient, skipping")
		_ = msg.Ack()
		return
	}

	var sendErr error
	switch data.EmailType {
	case events.EmailTypeUpcomingPayment:
		dueDate, _ := time.Parse(time.RFC3339, data.NextDue)
		sendErr = c.email.SendUpcomingPayment(data.To, data.Title, data.Amount, dueDate)
	case events.EmailTypeReceipt:
		nextDue, _ := time.Parse(time.RFC3339, data.NextDue)
		sendErr = c.email.SendReceipt(data.To, data.Title, data.Amount, data.TxHash, nextDue)
	case events.EmailTypePaymentReceipt:
		sendErr = c.email.SendPaymentReceipt(data.To, data.Title, data.Amount, data.TxHash)
	case events.EmailTypeFailed:
		sendErr = c.email.SendPaymentFailed(data.To, data.Title, data.Amount, data.Reason)
	case events.EmailTypeCancelled:
		sendErr = c.email.SendCancelled(data.To, data.Title)
	case events.EmailTypeExpired:
		sendErr = c.email.SendExpired(data.To, data.Title)
	case events.EmailTypeMerchantNotification:
		sendErr = c.email.SendMerchantPaymentNotification(data.To, data.Title, data.Amount, data.PayerAddress)
	default:
		log.Warn().Str("email_type", data.EmailType).Msg("email-sender: unknown email type")
		_ = msg.Ack()
		return
	}

	if sendErr != nil {
		log.Error().Err(sendErr).Str("email_type", data.EmailType).Str("to", data.To).Msg("email-sender: send failed, will retry")
		_ = msg.NakWithDelay(10 * time.Second)
		return
	}

	log.Info().Str("email_type", data.EmailType).Str("to", data.To).Msg("email sent")
	_ = msg.Ack()
}

func (c *Consumer) handlePaymentReceiptSend(ctx context.Context, msg jetstream.Msg) {
	evt, err := c.parseEvent(msg)
	if err != nil {
		log.Error().Err(err).Msg("payment-receipt: parse event")
		_ = msg.Nak()
		return
	}

	var data struct {
		PaymentID    string `json:"payment_id"`
		ShopID       string `json:"shop_id"`
		Kind         string `json:"kind"`
		TxHash       string `json:"tx_hash"`
		Amount       string `json:"amount"`
		PayerEmail   string `json:"payer_email"`
		PayerAddress string `json:"payer_address"`
		Title        string `json:"title"`
	}
	if err := json.Unmarshal(evt.Data, &data); err != nil {
		log.Error().Err(err).Msg("payment-receipt: unmarshal data")
		_ = msg.Nak()
		return
	}

	// Send receipt to payer
	if data.PayerEmail != "" {
		if err := c.email.SendPaymentReceipt(data.PayerEmail, data.Title, data.Amount, data.TxHash); err != nil {
			log.Error().Err(err).Str("payment_id", data.PaymentID).Str("to", data.PayerEmail).Msg("payment-receipt: send failed, will retry")
			_ = msg.NakWithDelay(10 * time.Second)
			return
		}
	}

	// Notify merchant
	merchantEmail, err := c.queries.GetMerchantEmail(ctx, data.ShopID)
	if err == nil && merchantEmail != nil && *merchantEmail != "" {
		if err := c.email.SendMerchantPaymentNotification(*merchantEmail, data.Title, data.Amount, data.PayerAddress); err != nil {
			log.Error().Err(err).Str("payment_id", data.PaymentID).Str("to", *merchantEmail).Msg("payment-receipt: merchant notify failed, will retry")
			_ = msg.NakWithDelay(10 * time.Second)
			return
		}
	}

	_ = msg.Ack()
}
