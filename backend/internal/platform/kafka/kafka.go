package kafka

import (
	"context"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/propagation"
)

// Message is the transport-agnostic Kafka message surface the application code
// works with. It deliberately hides the underlying franz-go type so handlers
// and tests do not depend on the client library directly.
type Message struct {
	Topic     string
	Partition int32
	Offset    int64
	Key       []byte
	Value     []byte
	Headers   []Header

	// leaderEpoch carries the originating record's leader epoch so Commit can
	// reconstruct a *kgo.Record precise enough for CommitRecords. It is zero for
	// synthetic messages built outside of Fetch (e.g. in tests), which never
	// reach the real *Consumer.Commit.
	leaderEpoch int32
}

// Header is a single Kafka message header.
type Header struct {
	Key   string
	Value []byte
}

// GetHeader returns the value of the first header with the given key, or "" if
// absent.
func (m Message) GetHeader(key string) string {
	for _, h := range m.Headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

// headerCarrier adapts []Header to the OTel TextMapCarrier interface so W3C
// traceparent/tracestate can be injected into and extracted from Kafka message
// headers.
type headerCarrier struct {
	headers *[]Header
}

func (c headerCarrier) Get(key string) string {
	for _, h := range *c.headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

func (c headerCarrier) Set(key, value string) {
	for i, h := range *c.headers {
		if h.Key == key {
			(*c.headers)[i].Value = []byte(value)
			return
		}
	}
	*c.headers = append(*c.headers, Header{Key: key, Value: []byte(value)})
}

func (c headerCarrier) Keys() []string {
	keys := make([]string, len(*c.headers))
	for i, h := range *c.headers {
		keys[i] = h.Key
	}
	return keys
}

var _ propagation.TextMapCarrier = headerCarrier{}

// toKgoHeaders converts our transport-agnostic headers to franz-go headers.
func toKgoHeaders(headers []Header) []kgo.RecordHeader {
	out := make([]kgo.RecordHeader, len(headers))
	for i, h := range headers {
		out[i] = kgo.RecordHeader{Key: h.Key, Value: h.Value}
	}
	return out
}

// fromKgoHeaders converts franz-go headers to our transport-agnostic ones.
func fromKgoHeaders(headers []kgo.RecordHeader) []Header {
	out := make([]Header, len(headers))
	for i, h := range headers {
		out[i] = Header{Key: h.Key, Value: h.Value}
	}
	return out
}

// MessageConsumer is the interface for consuming Kafka messages. Narrowed so
// tests can mock message consumption without a live broker.
// *Consumer satisfies it.
type MessageConsumer interface {
	Fetch(ctx context.Context) (Message, context.Context, error)
	Commit(ctx context.Context, messages ...Message) error
	HoldUntil(ctx context.Context, msg Message, untilUnixMillis int64) error
	Topic() string
	Close() error
}

// MessageProducer is the interface for producing Kafka messages. Narrowed so
// tests can mock message production and assert retry/DLQ routing without a live
// broker. *Producer satisfies it.
type MessageProducer interface {
	Publish(ctx context.Context, topic string, key, value []byte) error
	PublishWithHeaders(ctx context.Context, topic string, key, value []byte, headers []Header) error
	PublishDLQ(ctx context.Context, dlqTopic string, key, value []byte) error
	Close() error
}
