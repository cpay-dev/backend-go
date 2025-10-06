package transfers

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cpay-dev/proto-go/blockchain/v1/indexer"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"
)

type ConsumerConfig struct {
	URL         string `json:"url"`
	Stream      string `json:"stream"`
	Subject     string `json:"subject"`
	DurableName string `json:"durable_name"`
}

type Consumer struct {
	processor BlockProcessor
	config    ConsumerConfig
	logger    zerolog.Logger
	nats      *nats.Conn
	done      atomic.Bool
	stop      sync.WaitGroup
}

func NewConsumer(config ConsumerConfig, logger zerolog.Logger, processor BlockProcessor) *Consumer {
	return &Consumer{config: config, logger: logger, processor: processor}
}

func (c *Consumer) Run(ctx context.Context) error {
	c.stop.Add(1)
	defer c.stop.Done()

	client, err := nats.Connect(c.config.URL, nats.Secure(&tls.Config{}), nats.TLSHandshakeFirst())
	if err != nil {
		return fmt.Errorf("nats connect: %w", err)
	}
	c.nats = client

	js, err := jetstream.New(client)
	if err != nil {
		return fmt.Errorf("nats jetstream: %w", err)
	}

	consumer, err := js.CreateConsumer(ctx, c.config.Stream, jetstream.ConsumerConfig{
		FilterSubject: c.config.Subject,
		Durable:       c.config.DurableName,
	})
	if err != nil {
		return fmt.Errorf("jetstream create consumer: %w", err)
	}
	c.logger.Info().Str("stream", c.config.Stream).Str("subject", c.config.Subject).Msg("subscribed to stream")

	return c.run(ctx, consumer)
}

func (c *Consumer) Stop() {
	c.logger.Debug().Msg("stopping consumer...")
	c.done.Store(true)
	c.stop.Wait()
	if c.nats != nil {
		c.logger.Debug().Msg("closing nats...")
		if err := c.nats.Drain(); err != nil {
			c.logger.Err(err).Msg("drain nats failed")
		}
		c.nats.Close()
		c.logger.Debug().Msg("nats closed")
	}
	c.logger.Debug().Msg("consumer stopped")
}

func (c *Consumer) run(ctx context.Context, consumer jetstream.Consumer) error {
	for {
		if c.done.Load() {
			return nil
		}
		batch, err := consumer.Fetch(100, jetstream.FetchMaxWait(10*time.Second))
		if err != nil {
			return fmt.Errorf("jetstream fetch batch: %w", err)
		}
		if err := c.processBatch(ctx, batch); err != nil {
			return err
		}
	}
}

func (c *Consumer) processBatch(ctx context.Context, batch jetstream.MessageBatch) error {
	for {
		if c.done.Load() {
			return nil
		}
		msg, ok := <-batch.Messages()
		if !ok {
			if err := batch.Error(); err != nil {
				return fmt.Errorf("jetstream batch error: %w", err)
			}
			return nil
		}
		if err := c.processMessage(ctx, msg); err != nil {
			return fmt.Errorf("process message: %w", err)
		}
	}
}

func (c *Consumer) processMessage(ctx context.Context, msg jetstream.Msg) error {
	c.logger.Debug().Msg("unmarshalling block message")
	var parsedBlock indexer.ParsedBlock
	if err := proto.Unmarshal(msg.Data(), &parsedBlock); err != nil {
		return fmt.Errorf("unmarshal block message: %w", err)
	}
	c.logger.Debug().Msg("block message unmarshalled")

	c.logger.Debug().Msg("processing block")
	if err := c.processor.Process(ctx, &parsedBlock); err != nil {
		return fmt.Errorf("process block: %w", err)
	}
	c.logger.Debug().Msg("block processed")

	c.logger.Debug().Msg("acknowledging message")
	if err := msg.Ack(); err != nil {
		return fmt.Errorf("jetstream ack message: %w", err)
	}
	c.logger.Info().Uint64("block_number", parsedBlock.Block.BlockNumber).Msg("message acknowledged")

	return nil
}
