package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type Publisher struct {
	nc *nats.Conn
	js jetstream.JetStream
}

func NewPublisher(natsURL string) (*Publisher, error) {
	nc, err := nats.Connect(natsURL,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to nats: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("create jetstream: %w", err)
	}

	p := &Publisher{nc: nc, js: js}

	if err := p.initStreams(context.Background()); err != nil {
		nc.Close()
		return nil, fmt.Errorf("init streams: %w", err)
	}

	slog.Info("nats publisher connected", "url", natsURL)
	return p, nil
}

func (p *Publisher) initStreams(ctx context.Context) error {
	streams := []struct {
		name     string
		subjects []string
	}{
		{name: "USERS", subjects: []string{SubjectUsers + ".*"}},
		{name: "SHOPS", subjects: []string{SubjectShops + ".*"}},
		{name: "PRODUCTS", subjects: []string{SubjectProducts + ".*"}},
	}

	for _, s := range streams {
		_, err := p.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
			Name:      s.name,
			Subjects:  s.subjects,
			Retention: jetstream.WorkQueuePolicy,
			MaxAge:    7 * 24 * time.Hour,
		})
		if err != nil {
			return fmt.Errorf("create stream %s: %w", s.name, err)
		}
		slog.Info("jetstream stream ready", "stream", s.name)
	}

	return nil
}

func (p *Publisher) Publish(ctx context.Context, event *Event) error {
	eventBytes, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	subject := fmt.Sprintf("%s.%s", event.Subject, event.Type)
	_, err = p.js.Publish(ctx, subject, eventBytes)
	if err != nil {
		return fmt.Errorf("publish to %s: %w", subject, err)
	}

	slog.Debug("event published", "type", event.Type, "subject", subject, "id", event.ID)
	return nil
}

func (p *Publisher) Close() {
	if p.nc != nil {
		p.nc.Close()
	}
}
