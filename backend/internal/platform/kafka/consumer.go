package kafka

import (
	"context"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"transx/internal/platform/config"
)

type ConsumerOptions struct {
	Topic string
	Group string
}

// Consumer is a single-topic franz-go consumer with manual offset commit. It
// additionally exposes an internal pause/resume so HoldUntil can hold a
// message until its scheduled time without losing its place in the partition.
//
// A Consumer is driven by exactly one goroutine (the worker consume loop), so
// Fetch and HoldUntil never run concurrently for the same Consumer.
type Consumer struct {
	client     *kgo.Client
	topic      string
	group      string
	propagator propagation.TextMapPropagator
}

func NewConsumer(cfg config.Kafka, opts ConsumerOptions) *Consumer {
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ConsumerGroup(opts.Group),
		kgo.ConsumeTopics(opts.Topic),
		// Manual commit: the worker decides when an offset is durably handled so
		// a crash mid-handler redelivers the message.
		kgo.DisableAutoCommit(),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		panic(fmt.Sprintf("kafka: create consumer: %v", err))
	}
	return &Consumer{
		client:     cl,
		topic:      opts.Topic,
		group:      opts.Group,
		propagator: otel.GetTextMapPropagator(),
	}
}

func (c *Consumer) Topic() string { return c.topic }
func (c *Consumer) Group() string { return c.group }

// Fetch returns the next message together with a context carrying the trace
// extracted from the message headers. Callers should use the returned context
// (not the original ctx) for downstream DB/gRPC work so spans nest under the
// producer's trace. It blocks until a message arrives or ctx is cancelled.
func (c *Consumer) Fetch(ctx context.Context) (Message, context.Context, error) {
	for {
		if err := ctx.Err(); err != nil {
			return Message{}, ctx, err
		}
		fetches := c.client.PollFetches(ctx)
		if err := ctx.Err(); err != nil {
			return Message{}, ctx, err
		}
		if errs := fetches.Errors(); len(errs) > 0 {
			fe := errs[0]
			return Message{}, ctx, fmt.Errorf("kafka consumer error: %w", fe.Err)
		}

		var msg Message
		found := false
		fetches.EachRecord(func(r *kgo.Record) {
			if found {
				return
			}
			found = true
			msg = Message{
				Topic:       r.Topic,
				Partition:   r.Partition,
				Offset:      r.Offset,
				Key:         r.Key,
				Value:       r.Value,
				Headers:     fromKgoHeaders(r.Headers),
				leaderEpoch: r.LeaderEpoch,
			}
		})
		if !found {
			continue
		}
		carrier := headerCarrier{headers: &msg.Headers}
		return msg, c.propagator.Extract(ctx, carrier), nil
	}
}

// Commit durably records that the given messages have been handled by
// committing their offsets. CommitRecords commits offset+1 per record
// internally, matching the previous at-least-once semantics.
func (c *Consumer) Commit(ctx context.Context, messages ...Message) error {
	if len(messages) == 0 {
		return nil
	}
	records := make([]*kgo.Record, len(messages))
	for i, m := range messages {
		records[i] = &kgo.Record{
			Topic:       m.Topic,
			Partition:   m.Partition,
			Offset:      m.Offset,
			LeaderEpoch: m.leaderEpoch,
		}
	}
	if err := c.client.CommitRecords(ctx, records...); err != nil {
		return fmt.Errorf("commit %d offset(s): %w", len(messages), err)
	}
	return nil
}

// pause stops fetching from the message's partition without committing, so
// HoldUntil can hold position while it waits out a delay.
func (c *Consumer) pause(m Message) {
	c.client.PauseFetchPartitions(map[string][]int32{m.Topic: {m.Partition}})
}

// resume restarts fetching from the message's partition.
func (c *Consumer) resume(m Message) {
	c.client.ResumeFetchPartitions(map[string][]int32{m.Topic: {m.Partition}})
}

func (c *Consumer) Close() error {
	c.client.Close()
	return nil
}

// HoldUntil pauses the message's partition and waits until untilUnixMillis is
// reached or ctx is cancelled, then resumes the partition. franz-go's
// consumer-group heartbeat runs on its own background goroutine independent of
// PollFetches, so unlike a poll-driven client this can block on a plain timer
// during the wait without risking a session timeout. Returns ctx.Err() if
// cancelled while waiting.
func (c *Consumer) HoldUntil(ctx context.Context, m Message, untilUnixMillis int64) error {
	c.pause(m)
	defer c.resume(m)

	remaining := time.Until(time.UnixMilli(untilUnixMillis))
	if remaining <= 0 {
		return nil
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
