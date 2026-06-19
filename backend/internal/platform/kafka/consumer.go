package kafka

import (
	"context"
	"fmt"
	"time"

	ckafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"transx/internal/platform/config"
)

// pollTimeoutMs bounds a single Poll so the consume loop can observe context
// cancellation promptly between polls.
const pollTimeoutMs = 500

type ConsumerOptions struct {
	Topic string
	Group string
}

// Consumer is a single-topic confluent-kafka-go consumer with manual offset
// commit. It additionally exposes Pause/Resume/Seek so the delayed-retry
// consumers can hold a message until its scheduled time without losing it.
//
// A Consumer is driven by exactly one goroutine (the worker consume loop), so
// the unsynchronised buffer below is safe: Fetch and HoldUntil never run
// concurrently for the same Consumer.
type Consumer struct {
	consumer   *ckafka.Consumer
	topic      string
	group      string
	propagator propagation.TextMapPropagator
	// buffer holds messages that Poll returned while a partition was being
	// paused. librdkafka's Pause is asynchronous and the local prefetch queue
	// may still yield records during HoldUntil; buffering instead of discarding
	// them prevents message loss. Fetch drains this before polling.
	buffer []*ckafka.Message
}

func NewConsumer(cfg config.Kafka, opts ConsumerOptions) *Consumer {
	c, err := ckafka.NewConsumer(&ckafka.ConfigMap{
		"bootstrap.servers": joinBrokers(cfg.Brokers),
		"group.id":          opts.Group,
		"auto.offset.reset": "earliest",
		// Manual commit: the worker decides when an offset is durably handled so
		// a crash mid-handler redelivers the message.
		"enable.auto.commit": false,
	})
	if err != nil {
		panic(fmt.Sprintf("kafka: create consumer: %v", err))
	}
	if err := c.Subscribe(opts.Topic, nil); err != nil {
		panic(fmt.Sprintf("kafka: subscribe %s: %v", opts.Topic, err))
	}
	return &Consumer{
		consumer:   c,
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
		// Drain any messages buffered while a partition was being paused before
		// polling the broker, so they are never lost or reordered.
		var ev ckafka.Event
		if len(c.buffer) > 0 {
			ev = c.buffer[0]
			c.buffer = c.buffer[1:]
		} else {
			ev = c.consumer.Poll(pollTimeoutMs)
		}
		if ev == nil {
			continue
		}
		switch e := ev.(type) {
		case *ckafka.Message:
			msg := Message{
				Topic:     topicName(e.TopicPartition.Topic),
				Partition: e.TopicPartition.Partition,
				Offset:    int64(e.TopicPartition.Offset),
				Key:       e.Key,
				Value:     e.Value,
				Headers:   fromCKafkaHeaders(e.Headers),
			}
			carrier := headerCarrier{headers: &msg.Headers}
			return msg, c.propagator.Extract(ctx, carrier), nil
		case ckafka.Error:
			// Transient errors (e.g. broker unreachable) are surfaced to the
			// caller, which logs and continues. Fatal errors stop the loop.
			if e.IsFatal() {
				return Message{}, ctx, fmt.Errorf("kafka consumer fatal error: %w", e)
			}
			return Message{}, ctx, fmt.Errorf("kafka consumer error: %w", e)
		default:
			// Rebalance and other informational events: keep polling.
			continue
		}
	}
}

// Commit durably records that the given message has been handled by committing
// its offset (offset+1) for the message's partition.
func (c *Consumer) Commit(ctx context.Context, messages ...Message) error {
	for _, m := range messages {
		tp := ckafka.TopicPartition{
			Topic:     topicPtr(m.Topic),
			Partition: m.Partition,
			Offset:    ckafka.Offset(m.Offset + 1),
		}
		if _, err := c.consumer.CommitOffsets([]ckafka.TopicPartition{tp}); err != nil {
			return fmt.Errorf("commit offset %s[%d]@%d: %w", m.Topic, m.Partition, m.Offset, err)
		}
	}
	return nil
}

// Pause stops fetching from the message's partition without committing, so a
// delayed-retry consumer can hold position while it waits out the delay.
func (c *Consumer) Pause(m Message) error {
	return c.consumer.Pause([]ckafka.TopicPartition{partitionOf(m)})
}

// Resume restarts fetching from the message's partition.
func (c *Consumer) Resume(m Message) error {
	return c.consumer.Resume([]ckafka.TopicPartition{partitionOf(m)})
}

// Seek rewinds the partition to the message's own offset so the next Poll
// re-delivers it. Paired with Pause/Resume to re-read a not-yet-due message.
func (c *Consumer) Seek(m Message) error {
	tp := ckafka.TopicPartition{
		Topic:     topicPtr(m.Topic),
		Partition: m.Partition,
		Offset:    ckafka.Offset(m.Offset),
	}
	_, err := c.consumer.SeekPartitions([]ckafka.TopicPartition{tp})
	return err
}

func (c *Consumer) Close() error {
	return c.consumer.Close()
}

// HoldUntil pauses the message's partition and waits until untilUnixMillis is
// reached or ctx is cancelled. It keeps calling Poll on the (paused) consumer so
// librdkafka's max.poll.interval.ms is not exceeded during long delays — paused
// partitions yield no messages, so the polls only serve to keep group
// membership alive. The partition is resumed before returning. Returns
// ctx.Err() if cancelled while waiting.
func (c *Consumer) HoldUntil(ctx context.Context, m Message, untilUnixMillis int64) error {
	if err := c.Pause(m); err != nil {
		return fmt.Errorf("pause partition %s[%d]: %w", m.Topic, m.Partition, err)
	}
	defer func() { _ = c.Resume(m) }()

	for {
		remaining := time.Until(time.UnixMilli(untilUnixMillis))
		if remaining <= 0 {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		// Poll keeps the consumer in the group; a paused partition normally
		// returns no records. But Pause is asynchronous, so the local prefetch
		// queue may still yield a record here — buffer it instead of dropping it
		// so Fetch can return it once the hold completes.
		wait := remaining
		if wait > maxHoldPoll {
			wait = maxHoldPoll
		}
		if msg, ok := c.consumer.Poll(int(wait.Milliseconds())).(*ckafka.Message); ok {
			c.buffer = append(c.buffer, msg)
		}
	}
}

// maxHoldPoll caps a single Poll during HoldUntil so context cancellation is
// observed reasonably promptly and max.poll.interval.ms is never approached.
const maxHoldPoll = 2 * time.Second

func partitionOf(m Message) ckafka.TopicPartition {
	return ckafka.TopicPartition{Topic: topicPtr(m.Topic), Partition: m.Partition}
}

func topicPtr(t string) *string { return &t }

func topicName(t *string) string {
	if t == nil {
		return ""
	}
	return *t
}
