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

// Producer publishes messages to Kafka, injecting the active W3C trace into
// each message's headers so downstream consumers can continue the trace.
type Producer struct {
	client     *kgo.Client
	propagator propagation.TextMapPropagator
}

// NewProducer creates a franz-go producer. All-ISR acks plus franz-go's
// default idempotent production keep the at-least-once, no-duplicate delivery
// guarantee the worker pipeline relies on.
func NewProducer(cfg config.Kafka) *Producer {
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.RequiredAcks(kgo.AllISRAcks()),
	)
	if err != nil {
		// A misconfigured producer is a programmer/deploy error; fail loud at
		// startup rather than returning an error every caller must thread.
		panic(fmt.Sprintf("kafka: create producer: %v", err))
	}
	return &Producer{client: cl, propagator: otel.GetTextMapPropagator()}
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

	rec := &kgo.Record{
		Topic:   topic,
		Key:     key,
		Value:   value,
		Headers: toKgoHeaders(hdrs),
	}

	// Synchronous publish: block until the broker acks (or ctx is cancelled) so
	// callers can decide commit/redelivery on the result.
	results := p.client.ProduceSync(ctx, rec)
	if err := results.FirstErr(); err != nil {
		return fmt.Errorf("produce to %s: %w", topic, err)
	}
	return nil
}

// PublishDLQ writes a failed message to the given per-service DLQ topic.
func (p *Producer) PublishDLQ(ctx context.Context, dlqTopic string, key, value []byte) error {
	return p.Publish(ctx, dlqTopic, key, value)
}

// Close flushes pending messages and shuts down the producer.
func (p *Producer) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = p.client.Flush(ctx)
	p.client.Close()
	return nil
}
