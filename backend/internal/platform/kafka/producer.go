package kafka

import (
	"context"
	"fmt"

	ckafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"transx/internal/platform/config"
)

// Producer publishes messages to Kafka, injecting the active W3C trace into
// each message's headers so downstream consumers can continue the trace.
type Producer struct {
	producer   *ckafka.Producer
	propagator propagation.TextMapPropagator
}

// NewProducer creates a confluent-kafka-go producer. RequireAll acks keep the
// at-least-once delivery guarantee the worker pipeline relies on.
func NewProducer(cfg config.Kafka) *Producer {
	p, err := ckafka.NewProducer(&ckafka.ConfigMap{
		"bootstrap.servers": joinBrokers(cfg.Brokers),
		"acks":              "all",
		// Idempotent producer dedups retries inside a single producer session so
		// republished retries do not create duplicate records on the broker.
		"enable.idempotence": true,
	})
	if err != nil {
		// A misconfigured producer is a programmer/deploy error; fail loud at
		// startup rather than returning an error every caller must thread.
		panic(fmt.Sprintf("kafka: create producer: %v", err))
	}
	return &Producer{producer: p, propagator: otel.GetTextMapPropagator()}
}

// Publish writes a message to topic, injecting the W3C traceparent header from
// ctx so downstream consumers can continue the trace.
func (p *Producer) Publish(ctx context.Context, topic string, key, value []byte) error {
	return p.PublishWithHeaders(ctx, topic, key, value, nil)
}

// PublishWithHeaders writes a message with caller-supplied headers, additionally
// injecting the active trace context. Used by the delayed-retry machinery to
// carry attempt/retry-at/origin headers alongside the trace.
func (p *Producer) PublishWithHeaders(
	ctx context.Context,
	topic string,
	key, value []byte,
	headers []Header,
) error {
	hdrs := make([]Header, len(headers))
	copy(hdrs, headers)
	p.propagator.Inject(ctx, headerCarrier{headers: &hdrs})

	deliveryCh := make(chan ckafka.Event, 1)
	msg := &ckafka.Message{
		TopicPartition: ckafka.TopicPartition{Topic: &topic, Partition: ckafka.PartitionAny},
		Key:            key,
		Value:          value,
		Headers:        toCKafkaHeaders(hdrs),
	}
	if err := p.producer.Produce(msg, deliveryCh); err != nil {
		return fmt.Errorf("produce to %s: %w", topic, err)
	}

	// Synchronous publish: block until the broker acks (or ctx is cancelled) so
	// callers can decide commit/redelivery on the result, matching the previous
	// segmentio WriteMessages semantics.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case ev := <-deliveryCh:
		m, ok := ev.(*ckafka.Message)
		if !ok {
			return fmt.Errorf("produce to %s: unexpected delivery event %T", topic, ev)
		}
		if m.TopicPartition.Error != nil {
			return fmt.Errorf("produce to %s: %w", topic, m.TopicPartition.Error)
		}
		return nil
	}
}

// PublishDLQ writes a failed message to the given per-service DLQ topic.
func (p *Producer) PublishDLQ(ctx context.Context, dlqTopic string, key, value []byte) error {
	return p.Publish(ctx, dlqTopic, key, value)
}

// Close flushes pending messages and shuts down the producer.
func (p *Producer) Close() error {
	p.producer.Flush(5000)
	p.producer.Close()
	return nil
}

func joinBrokers(brokers []string) string {
	out := ""
	for i, b := range brokers {
		if i > 0 {
			out += ","
		}
		out += b
	}
	return out
}
