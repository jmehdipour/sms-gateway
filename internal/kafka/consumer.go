package kafka

import (
	"context"
	"time"

	"github.com/segmentio/kafka-go"
)

type Config struct {
	Brokers        []string
	Topic          string
	GroupID        string
	MinBytes       int           // default 1KB
	MaxBytes       int           // default 10MB
	CommitInterval time.Duration // default 1s (0 = sync each msg)
	MaxWait        time.Duration // default 1s (0 = sync each msg)
}

// Consumer is a thin wrapper around segmentio/kafka-go Reader.
type Consumer struct {
	r *kafka.Reader
}

func NewConsumerFromConfig(c Config) *Consumer {
	min := c.MinBytes
	if min <= 0 {
		min = 1 << 10 // 1KB
	}
	max := c.MaxBytes
	if max <= 0 {
		max = 10 << 20 // 10MB
	}
	ci := c.CommitInterval
	if ci <= 0 {
		ci = time.Second
	}

	mw := c.MaxWait
	if mw <= 0 {
		mw = 50 * time.Millisecond
	}

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        c.Brokers,
		GroupID:        c.GroupID,
		Topic:          c.Topic,
		MinBytes:       min,
		MaxBytes:       max,
		CommitInterval: ci,
		MaxWait:        mw,
	})

	return &Consumer{r: r}
}

type Message = kafka.Message

func (c *Consumer) Fetch(ctx context.Context) (Message, error) {
	return c.r.FetchMessage(ctx)
}

func (c *Consumer) Commit(ctx context.Context, m Message) error {
	return c.r.CommitMessages(ctx, m)
}

func (c *Consumer) Close() error { return c.r.Close() }
