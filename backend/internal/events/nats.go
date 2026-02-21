package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rs/zerolog/log"
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

	log.Info().Str("url", natsURL).Msg("nats publisher connected")
	return p, nil
}

func (p *Publisher) initStreams(ctx context.Context) error {
	streams := []struct {
		name     string
		subjects []string
	}{
		{name: "USERS", subjects: []string{SubjectUsers + ".>"}},
		{name: "SHOPS", subjects: []string{SubjectShops + ".>"}},
		{name: "PRODUCTS", subjects: []string{SubjectProducts + ".>"}},
		{name: "PAYMENT_LINKS", subjects: []string{SubjectPaymentLinks + ".>"}},
		{name: "PAYMENTS", subjects: []string{SubjectPayments + ".>"}},
		{name: "SUBSCRIPTIONS", subjects: []string{SubjectSubscriptions + ".>"}},
		{name: "SUBSCRIBERS", subjects: []string{SubjectSubscribers + ".>"}},
		{name: "EMAILS", subjects: []string{SubjectEmails + ".>"}},
	}

	for _, s := range streams {
		cfg := jetstream.StreamConfig{
			Name:      s.name,
			Subjects:  s.subjects,
			Retention: jetstream.InterestPolicy,
			MaxAge:    7 * 24 * time.Hour,
		}
		_, err := p.js.CreateOrUpdateStream(ctx, cfg)
		if err != nil {
			// Retention policy is immutable — delete and recreate if it changed.
			if delErr := p.js.DeleteStream(ctx, s.name); delErr != nil {
				return fmt.Errorf("create stream %s: %w (delete attempt: %v)", s.name, err, delErr)
			}
			log.Warn().Str("stream", s.name).Msg("deleted stream with stale config, recreating")
			if _, err = p.js.CreateOrUpdateStream(ctx, cfg); err != nil {
				return fmt.Errorf("recreate stream %s: %w", s.name, err)
			}
		}
		log.Info().Str("stream", s.name).Msg("jetstream stream ready")
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

	log.Debug().Str("type", event.Type).Str("subject", subject).Str("id", event.ID).Msg("event published")
	return nil
}

// JetStream returns the underlying JetStream context for creating consumers.
func (p *Publisher) JetStream() jetstream.JetStream {
	return p.js
}

func (p *Publisher) Close() {
	if p.nc != nil {
		p.nc.Close()
	}
}
