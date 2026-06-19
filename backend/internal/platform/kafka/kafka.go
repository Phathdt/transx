package kafka

import (
	ckafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"go.opentelemetry.io/otel/propagation"
)

// Message is the transport-agnostic Kafka message surface the application code
// works with. It deliberately hides the underlying confluent-kafka-go type so
// handlers and tests do not depend on the client library directly.
type Message struct {
	Topic     string
	Partition int32
	Offset    int64
	Key       []byte
	Value     []byte
	Headers   []Header
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

// toCKafkaHeaders converts our transport-agnostic headers to confluent headers.
func toCKafkaHeaders(headers []Header) []ckafka.Header {
	out := make([]ckafka.Header, len(headers))
	for i, h := range headers {
		out[i] = ckafka.Header{Key: h.Key, Value: h.Value}
	}
	return out
}

// fromCKafkaHeaders converts confluent headers to our transport-agnostic ones.
func fromCKafkaHeaders(headers []ckafka.Header) []Header {
	out := make([]Header, len(headers))
	for i, h := range headers {
		out[i] = Header{Key: h.Key, Value: h.Value}
	}
	return out
}
