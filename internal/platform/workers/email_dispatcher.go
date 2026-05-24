package workers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/cpay-dev/cpay/internal/platform/storage"
	"github.com/cpay-dev/cpay/internal/shared/events"
	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/resend/resend-go/v3"
	"github.com/rs/zerolog"
)

type EmailAttachment struct {
	Filename    string
	ContentType string
	Content     []byte
}

type EmailMessage struct {
	From        string
	ReplyTo     string
	To          []string
	Subject     string
	Text        string
	HTML        string
	Attachments []EmailAttachment
}

type EmailSender interface {
	Send(ctx context.Context, msg EmailMessage) (string, error)
}

type ResendSender struct {
	client *resend.Client
}

func NewResendSender(apiKey string) (*ResendSender, error) {
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return nil, errors.New("resend api key is empty")
	}
	return &ResendSender{client: resend.NewClient(key)}, nil
}

func (s *ResendSender) Send(ctx context.Context, msg EmailMessage) (string, error) {
	attachments := make([]*resend.Attachment, 0, len(msg.Attachments))
	for _, attachment := range msg.Attachments {
		attachments = append(attachments, &resend.Attachment{
			Filename:    attachment.Filename,
			ContentType: attachment.ContentType,
			Content:     attachment.Content,
		})
	}
	req := &resend.SendEmailRequest{
		From:        msg.From,
		To:          msg.To,
		Subject:     msg.Subject,
		Text:        msg.Text,
		Html:        msg.HTML,
		Attachments: attachments,
	}
	if strings.TrimSpace(msg.ReplyTo) != "" {
		req.ReplyTo = msg.ReplyTo
	}
	resp, err := s.client.Emails.SendWithContext(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Id, nil
}

type EmailDispatcher struct {
	DB           *pgxpool.Pool
	Log          zerolog.Logger
	Sender       EmailSender
	Minio        *storage.MinIO
	From         string
	ReplyTo      string
	PollInterval time.Duration
}

type paymentEventData struct {
	PaymentIntentID string `json:"payment_intent_id"`
}

type invoiceEventData struct {
	InvoiceID       string `json:"invoice_id"`
	PaymentIntentID string `json:"payment_intent_id"`
	ObjectKey       string `json:"object_key"`
	Amount          float64
	Currency        string
}

type userSignedUpData struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
}

type paymentEmailContext struct {
	PaymentIntentID string
	MerchantID      string
	MerchantEmail   string
	CustomerEmail   string
	LinkTitle       string
	Currency        string
	ExpectedAmount  float64
	ReceivedAmount  float64
	TxHash          string
	InvoiceObject   string
}

func (w *EmailDispatcher) Run(ctx context.Context, nc *nats.Conn) {
	if w.Sender == nil {
		w.Log.Warn().Msg("email dispatcher disabled: sender not configured")
		<-ctx.Done()
		return
	}
	if strings.TrimSpace(w.From) == "" {
		w.Log.Warn().Msg("email dispatcher disabled: EMAIL_FROM is empty")
		<-ctx.Done()
		return
	}
	if nc == nil {
		w.Log.Warn().Msg("email dispatcher started without nats subscription; durable outbox polling remains active")
	} else {
		_, err := nc.Subscribe("events.>", func(msg *nats.Msg) {
			msgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := w.handleIncoming(msgCtx, msg.Data); err != nil {
				w.Log.Error().Err(err).Msg("email dispatcher: handle event failed")
			}
		})
		if err != nil {
			w.Log.Error().Err(err).Msg("email dispatcher: failed to subscribe to nats")
			<-ctx.Done()
			return
		}
	}

	if w.PollInterval <= 0 {
		w.PollInterval = 10 * time.Second
	}
	ticker := time.NewTicker(w.PollInterval)
	defer ticker.Stop()

	w.dispatchOutboxEvents(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.dispatchOutboxEvents(ctx)
		}
	}
}

func (w *EmailDispatcher) dispatchOutboxEvents(ctx context.Context) {
	if w.DB == nil {
		return
	}

	rows, err := w.DB.Query(ctx, `
		SELECT payload::text
		FROM platform.outbox_events
		WHERE event_type = ANY($1::text[])
		ORDER BY created_at DESC
		LIMIT 200
	`, supportedEmailEventTypes())
	if err != nil {
		w.Log.Error().Err(err).Msg("email dispatcher: outbox query failed")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			w.Log.Error().Err(err).Msg("email dispatcher: outbox scan failed")
			continue
		}
		eventCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if err := w.handleIncoming(eventCtx, []byte(payload)); err != nil {
			w.Log.Error().Err(err).Msg("email dispatcher: outbox event failed")
		}
		cancel()
	}
	if err := rows.Err(); err != nil {
		w.Log.Error().Err(err).Msg("email dispatcher: outbox rows failed")
	}
}

func (w *EmailDispatcher) handleIncoming(ctx context.Context, payload []byte) error {
	var env events.Envelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return err
	}
	if !emailEventSupported(env.Type) {
		return nil
	}
	switch env.Type {
	case "user.signed_up":
		return w.handleUserSignedUp(ctx, env)
	case "invoice.created":
		return w.handleInvoiceCreated(ctx, env)
	case "payment.confirmed":
		return w.handlePaymentConfirmed(ctx, env)
	case "payment.expired", "payment.failed":
		return w.handlePaymentFailed(ctx, env)
	default:
		return nil
	}
}

func emailEventSupported(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "user.signed_up", "invoice.created", "payment.confirmed", "payment.expired", "payment.failed":
		return true
	default:
		return false
	}
}

func supportedEmailEventTypes() []string {
	return []string{"user.signed_up", "invoice.created", "payment.confirmed", "payment.expired", "payment.failed"}
}

func (w *EmailDispatcher) handleUserSignedUp(ctx context.Context, env events.Envelope) error {
	var data userSignedUpData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return err
	}
	email := strings.TrimSpace(strings.ToLower(data.Email))
	if email == "" && strings.TrimSpace(data.UserID) != "" {
		_ = w.DB.QueryRow(ctx, `SELECT lower(email) FROM auth.users WHERE id::text=$1`, strings.TrimSpace(data.UserID)).Scan(&email)
	}
	if email == "" {
		return nil
	}
	subject := "Welcome to cpay"
	text := "Welcome to cpay. Your account is ready, and you can start accepting payments."
	html := "<p>Welcome to <strong>cpay</strong>.</p><p>Your account is ready, and you can start accepting payments.</p>"
	return w.dispatch(ctx, env, "user_signup", []string{email}, subject, text, html, nil)
}

func (w *EmailDispatcher) handleInvoiceCreated(ctx context.Context, env events.Envelope) error {
	var data invoiceEventData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return err
	}
	intentID := strings.TrimSpace(data.PaymentIntentID)
	if intentID == "" {
		return nil
	}
	info, err := w.loadPaymentContext(ctx, intentID)
	if err != nil {
		return err
	}
	recipients := uniqueEmails([]string{info.CustomerEmail, info.MerchantEmail})
	if len(recipients) == 0 {
		return nil
	}

	objectKey := strings.TrimSpace(data.ObjectKey)
	if objectKey == "" {
		objectKey = info.InvoiceObject
	}
	attachments := w.loadInvoiceAttachment(ctx, objectKey)

	subject := fmt.Sprintf("Invoice for %s", info.LinkTitle)
	text := fmt.Sprintf("Your invoice is ready.\nPayment intent: %s\nAmount: %.8f %s", info.PaymentIntentID, amountForInvoice(data.Amount, info.ReceivedAmount), valueOrDefault(data.Currency, info.Currency))
	html := fmt.Sprintf(
		"<p>Your invoice is ready.</p><p><strong>Payment intent:</strong> %s<br><strong>Amount:</strong> %.8f %s</p>",
		info.PaymentIntentID, amountForInvoice(data.Amount, info.ReceivedAmount), valueOrDefault(data.Currency, info.Currency),
	)
	return w.dispatch(ctx, env, "invoice_created", recipients, subject, text, html, attachments)
}

func (w *EmailDispatcher) handlePaymentConfirmed(ctx context.Context, env events.Envelope) error {
	intentID := paymentIntentIDFromEvent(env.Data)
	if intentID == "" {
		return nil
	}
	info, err := w.loadPaymentContext(ctx, intentID)
	if err != nil {
		return err
	}
	recipients := uniqueEmails([]string{info.CustomerEmail, info.MerchantEmail})
	if len(recipients) == 0 {
		return nil
	}
	attachments := w.loadInvoiceAttachment(ctx, info.InvoiceObject)
	subject := fmt.Sprintf("Payment successful for %s", info.LinkTitle)
	text := fmt.Sprintf("Payment succeeded.\nAmount: %.8f %s\nTransaction: %s", info.ReceivedAmount, info.Currency, valueOrDefault(info.TxHash, "-"))
	html := fmt.Sprintf("<p>Payment succeeded.</p><p><strong>Amount:</strong> %.8f %s<br><strong>Transaction:</strong> %s</p>", info.ReceivedAmount, info.Currency, valueOrDefault(info.TxHash, "-"))
	return w.dispatch(ctx, env, "payment_success", recipients, subject, text, html, attachments)
}

func (w *EmailDispatcher) handlePaymentFailed(ctx context.Context, env events.Envelope) error {
	intentID := paymentIntentIDFromEvent(env.Data)
	if intentID == "" {
		return nil
	}
	info, err := w.loadPaymentContext(ctx, intentID)
	if err != nil {
		return err
	}
	recipients := uniqueEmails([]string{info.CustomerEmail, info.MerchantEmail})
	if len(recipients) == 0 {
		return nil
	}

	subject := fmt.Sprintf("Payment failed for %s", info.LinkTitle)
	text := fmt.Sprintf("Payment did not complete.\nExpected: %.8f %s\nReceived: %.8f %s", info.ExpectedAmount, info.Currency, info.ReceivedAmount, info.Currency)
	html := fmt.Sprintf("<p>Payment did not complete.</p><p><strong>Expected:</strong> %.8f %s<br><strong>Received:</strong> %.8f %s</p>", info.ExpectedAmount, info.Currency, info.ReceivedAmount, info.Currency)
	return w.dispatch(ctx, env, "payment_failed", recipients, subject, text, html, nil)
}

func (w *EmailDispatcher) dispatch(ctx context.Context, env events.Envelope, template string, recipients []string, subject, text, html string, attachments []EmailAttachment) error {
	for _, recipient := range recipients {
		reserved, err := w.reserveNotification(ctx, env, template, recipient)
		if err != nil {
			return err
		}
		if !reserved {
			continue
		}

		messageID, err := w.Sender.Send(ctx, EmailMessage{
			From:        w.From,
			ReplyTo:     w.ReplyTo,
			To:          []string{recipient},
			Subject:     subject,
			Text:        text,
			HTML:        html,
			Attachments: attachments,
		})
		if err != nil {
			_ = w.markNotificationFailed(ctx, env.ID, template, recipient, err.Error())
			continue
		}
		if err := w.markNotificationSent(ctx, env.ID, template, recipient, messageID); err != nil {
			w.Log.Error().Err(err).Str("event_id", env.ID).Str("template", template).Str("recipient", recipient).Msg("email dispatcher: mark sent failed")
		}
	}
	return nil
}

func (w *EmailDispatcher) reserveNotification(ctx context.Context, env events.Envelope, template, recipient string) (bool, error) {
	var merchantID any
	if v, err := ids.Parse(strings.TrimSpace(env.MerchantID)); err == nil {
		merchantID = v
	}
	cmd, err := w.DB.Exec(ctx, `
		INSERT INTO platform.email_notifications(
			id, event_id, event_type, merchant_id, template, recipient, status, attempts, created_at, updated_at
		)
		VALUES($1, $2, $3, $4, $5, $6, 'pending', 0, NOW(), NOW())
		ON CONFLICT (event_id, template, recipient) DO NOTHING
	`, ids.New(), env.ID, env.Type, merchantID, template, recipient)
	if err != nil {
		return false, err
	}
	return cmd.RowsAffected() > 0, nil
}

func (w *EmailDispatcher) markNotificationSent(ctx context.Context, eventID, template, recipient, providerMessageID string) error {
	_, err := w.DB.Exec(ctx, `
		UPDATE platform.email_notifications
		SET status='sent', provider_message_id=$4, attempts=attempts+1, sent_at=NOW(), last_error=NULL, updated_at=NOW()
		WHERE event_id=$1 AND template=$2 AND recipient=$3
	`, eventID, template, recipient, providerMessageID)
	return err
}

func (w *EmailDispatcher) markNotificationFailed(ctx context.Context, eventID, template, recipient, lastError string) error {
	_, err := w.DB.Exec(ctx, `
		UPDATE platform.email_notifications
		SET status='failed', last_error=$4, attempts=attempts+1, updated_at=NOW()
		WHERE event_id=$1 AND template=$2 AND recipient=$3
	`, eventID, template, recipient, lastError)
	return err
}

func (w *EmailDispatcher) loadPaymentContext(ctx context.Context, paymentIntentID string) (paymentEmailContext, error) {
	var out paymentEmailContext
	var expectedRaw, receivedRaw string
	err := w.DB.QueryRow(ctx, `
		SELECT i.id::text, i.merchant_id::text,
			COALESCE(c.customer_email, ''),
			COALESCE(mu.email, ''),
			COALESCE(p.title, 'Payment'),
			COALESCE(c.currency, ''),
			COALESCE(i.expected_amount::text, '0'),
			COALESCE(i.received_amount::text, '0'),
			COALESCE(i.tx_hash, ''),
			COALESCE(inv.object_key, '')
		FROM checkout.payment_intents i
		JOIN checkout.checkout_sessions c ON c.id=i.checkout_session_id
		JOIN catalog.payment_links p ON p.id=i.payment_link_id
		LEFT JOIN checkout.invoices inv ON inv.payment_intent_id=i.id
		LEFT JOIN LATERAL (
			SELECT u.email
			FROM auth.users u
			WHERE u.merchant_id=i.merchant_id
			ORDER BY CASE WHEN lower(u.role)='admin' THEN 0 ELSE 1 END, u.created_at
			LIMIT 1
		) mu ON TRUE
		WHERE i.id::text=$1
	`, strings.TrimSpace(paymentIntentID)).Scan(
		&out.PaymentIntentID,
		&out.MerchantID,
		&out.CustomerEmail,
		&out.MerchantEmail,
		&out.LinkTitle,
		&out.Currency,
		&expectedRaw,
		&receivedRaw,
		&out.TxHash,
		&out.InvoiceObject,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return paymentEmailContext{}, nil
		}
		return paymentEmailContext{}, err
	}
	out.ExpectedAmount = parseFloat(expectedRaw)
	out.ReceivedAmount = parseFloat(receivedRaw)
	if strings.TrimSpace(out.Currency) == "" {
		out.Currency = "USD"
	}
	return out, nil
}

func (w *EmailDispatcher) loadInvoiceAttachment(ctx context.Context, objectKey string) []EmailAttachment {
	if w.Minio == nil || strings.TrimSpace(objectKey) == "" {
		return nil
	}
	payload, contentType, err := w.Minio.GetObjectBytes(ctx, objectKey)
	if err != nil {
		w.Log.Warn().Err(err).Str("object_key", objectKey).Msg("email dispatcher: failed to load invoice object")
		return nil
	}
	filename := path.Base(objectKey)
	if strings.TrimSpace(filename) == "" {
		filename = "receipt.pdf"
	}
	return []EmailAttachment{{
		Filename:    filename,
		ContentType: contentType,
		Content:     payload,
	}}
}

func paymentIntentIDFromEvent(raw json.RawMessage) string {
	var data paymentEventData
	if err := json.Unmarshal(raw, &data); err != nil {
		return ""
	}
	return strings.TrimSpace(data.PaymentIntentID)
}

func uniqueEmails(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		email := strings.ToLower(strings.TrimSpace(value))
		if email == "" {
			continue
		}
		if _, exists := seen[email]; exists {
			continue
		}
		seen[email] = struct{}{}
		out = append(out, email)
	}
	return out
}

func valueOrDefault(v, fallback string) string {
	value := strings.TrimSpace(v)
	if value == "" {
		return fallback
	}
	return value
}

func amountForInvoice(amount, fallback float64) float64 {
	if amount > 0 {
		return amount
	}
	return fallback
}
