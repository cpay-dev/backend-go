package bus

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

func Connect(url string) (*nats.Conn, error) {
	conn, err := nats.Connect(url,
		nats.Name("cpay"),
		nats.Timeout(5*time.Second),
		nats.ReconnectWait(2*time.Second),
		nats.MaxReconnects(-1),
	)
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}
	return conn, nil
}

func PublishJSON(_ context.Context, nc *nats.Conn, subject string, payload any) error {
	if nc == nil {
		return fmt.Errorf("nats not initialized")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal nats payload: %w", err)
	}
	if err = nc.Publish(subject, body); err != nil {
		return fmt.Errorf("publish %s: %w", subject, err)
	}
	return nil
}
