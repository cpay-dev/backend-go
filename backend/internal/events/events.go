package events

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nuid"
)

const (
	SubjectUsers         = "users"
	SubjectShops         = "shops"
	SubjectProducts      = "products"
	SubjectPaymentLinks  = "payment_links"
	SubjectPayments      = "payments"
	SubjectSubscriptions = "subscriptions"
	SubjectSubscribers   = "subscribers"
	SubjectEmails        = "emails"
)

const (
	EventUserCreated = "user.created"
	EventUserUpdated = "user.updated"

	EventShopCreated = "shop.created"
	EventShopUpdated = "shop.updated"

	EventProductCreated = "product.created"
	EventProductUpdated = "product.updated"

	EventPaymentLinkCreated = "payment_link.created"
	EventPaymentLinkUsed    = "payment_link.used"

	EventPaymentRecorded = "payment.recorded"

	EventSubscriptionCreated = "subscription.created"
	EventSubscriptionUpdated = "subscription.updated"
	EventSubscriptionPayment = "subscription.payment"

	EventSubscriberCreated   = "subscriber.created"
	EventSubscriberCancelled = "subscriber.cancelled"
	EventSubscriberExpired   = "subscriber.expired"
	EventSubscriberCharged   = "subscriber.charged"
	EventSubscriberFailed    = "subscriber.failed"

	EventEmailQueued = "email.queued"
	EventEmailSent   = "email.sent"
	EventEmailFailed = "email.failed"
)

type Event struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Subject   string          `json:"subject"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// Email types for EventEmailQueued payloads.
const (
	EmailTypeUpcomingPayment      = "upcoming_payment"
	EmailTypeReceipt              = "receipt"
	EmailTypePaymentReceipt       = "payment_receipt"
	EmailTypeFailed               = "failed"
	EmailTypeCancelled            = "cancelled"
	EmailTypeExpired              = "expired"
	EmailTypeMerchantNotification = "merchant_notification"
)

// EmailQueuedData is the payload for EventEmailQueued events.
type EmailQueuedData struct {
	EmailType    string `json:"email_type"`
	To           string `json:"to"`
	Title        string `json:"title"`
	Amount       string `json:"amount,omitempty"`
	TxHash       string `json:"tx_hash,omitempty"`
	NextDue      string `json:"next_due,omitempty"`
	Reason       string `json:"reason,omitempty"`
	PayerAddress string `json:"payer_address,omitempty"`
}

func NewEvent(eventType, subject string, data interface{}) (*Event, error) {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal event data: %w", err)
	}
	return &Event{
		ID:        nuid.Next(),
		Type:      eventType,
		Subject:   subject,
		Timestamp: time.Now().UTC(),
		Data:      dataBytes,
	}, nil
}
