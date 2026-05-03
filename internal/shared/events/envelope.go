package events

import (
	"encoding/json"
	"time"

	"github.com/cpay-dev/cpay/internal/shared/ids"
)

type Envelope struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Version     string          `json:"version"`
	OccurredAt  time.Time       `json:"occurred_at"`
	Source      string          `json:"source"`
	MerchantID  string          `json:"merchant_id,omitempty"`
	AggregateID string          `json:"aggregate_id,omitempty"`
	Data        json.RawMessage `json:"data"`
}

func NewEnvelope(eventType, source, merchantID, aggregateID string, payload any) (Envelope, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		ID:          ids.New(),
		Type:        eventType,
		Version:     "v1",
		OccurredAt:  time.Now().UTC(),
		Source:      source,
		MerchantID:  merchantID,
		AggregateID: aggregateID,
		Data:        body,
	}, nil
}
