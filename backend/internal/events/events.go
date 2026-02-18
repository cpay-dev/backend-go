package events

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nuid"
)

const (
	SubjectUsers              = "users"
	SubjectShops              = "shops"
	SubjectProducts           = "products"
	SubjectPaymentLinks       = "payment_links"
	SubjectPayments           = "payments"
	SubjectSubscriptions      = "subscriptions"
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
)

type Event struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Subject   string          `json:"subject"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
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
