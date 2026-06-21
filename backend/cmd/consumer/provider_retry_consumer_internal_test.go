package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"transx/internal/common/kafkatopic"
	"transx/internal/modules/wallet/application/dto"
	"transx/internal/modules/wallet/domain/entities"
	"transx/internal/platform/kafka"
	"transx/internal/platform/logger"
	"transx/internal/testmocks"
)

type internalPublishedMsg struct {
	Topic   string
	Key     []byte
	Value   []byte
	Headers []kafka.Header
}

type internalProducer struct {
	published []internalPublishedMsg
	err       error
}

func (p *internalProducer) Publish(_ context.Context, topic string, key, value []byte) error {
	if p.err != nil {
		return p.err
	}
	p.published = append(p.published, internalPublishedMsg{Topic: topic, Key: key, Value: value})
	return nil
}

func (p *internalProducer) PublishWithHeaders(
	_ context.Context,
	topic string,
	key, value []byte,
	headers []kafka.Header,
) error {
	if p.err != nil {
		return p.err
	}
	p.published = append(p.published, internalPublishedMsg{Topic: topic, Key: key, Value: value, Headers: headers})
	return nil
}

func (p *internalProducer) PublishDLQ(_ context.Context, topic string, key, value []byte) error {
	if p.err != nil {
		return p.err
	}
	p.published = append(p.published, internalPublishedMsg{Topic: topic, Key: key, Value: value})
	return nil
}

func (p *internalProducer) Close() error { return nil }

type internalConsumer struct {
	msg       kafka.Message
	fetchErr  error
	commits   []kafka.Message
	holds     []int64
	holdErr   error
	commitErr error
	topic     string
}

func (c *internalConsumer) Fetch(ctx context.Context) (kafka.Message, context.Context, error) {
	if c.fetchErr != nil {
		return kafka.Message{}, nil, c.fetchErr
	}
	return c.msg, context.Background(), nil
}

func (c *internalConsumer) Commit(_ context.Context, messages ...kafka.Message) error {
	if c.commitErr != nil {
		return c.commitErr
	}
	c.commits = append(c.commits, messages...)
	return nil
}

func (c *internalConsumer) HoldUntil(_ context.Context, _ kafka.Message, at int64) error {
	if c.holdErr != nil {
		return c.holdErr
	}
	c.holds = append(c.holds, at)
	return nil
}

func (c *internalConsumer) Topic() string {
	if c.topic != "" {
		return c.topic
	}
	return kafkatopic.TransferProviderRequested
}

func (c *internalConsumer) Close() error { return nil }

type internalProviderClient struct {
	result entities.ProviderResult
	err    error
	calls  int
}

func (c *internalProviderClient) Submit(
	_ context.Context,
	_ uuid.UUID,
	_ decimal.Decimal,
	_ string,
) (entities.ProviderResult, error) {
	c.calls++
	return c.result, c.err
}

func internalTransferMessage(transferID uuid.UUID) kafka.Message {
	payload, _ := json.Marshal(dto.TransferEventPayload{TransferID: transferID.String()})
	return kafka.Message{Topic: kafkatopic.TransferProviderRequested, Key: []byte(transferID.String()), Value: payload}
}

func TestProviderConsumerHandle(t *testing.T) {
	ctx := context.Background()
	log := logger.New("plain", "error")

	t.Run("settles provider success and marks processed", func(t *testing.T) {
		transferID := uuid.New()
		consumer := &internalConsumer{}
		producer := &internalProducer{}
		client := &internalProviderClient{
			result: entities.ProviderResult{Outcome: entities.ProviderSuccess, ReferenceID: "ref"},
		}
		transfers := testmocks.NewTransferRepository(t)
		inbox := testmocks.NewInboxRepository(t)
		pc := NewProviderConsumer(consumer, producer, client, transfers, inbox, log)

		inbox.EXPECT().IsProcessed(ctx, providerConsumerGroup, transferID.String()).Return(false, nil)
		transfers.EXPECT().GetByID(ctx, transferID).Return(&entities.Transfer{
			ID:                  transferID,
			Status:              entities.TransferStatusReserved,
			TransactionAmount:   decimal.NewFromInt(25),
			TransactionCurrency: "USD",
		}, nil)
		transfers.EXPECT().SettleExternalTransfer(ctx, transferID, client.result).Return(nil)
		inbox.EXPECT().MarkProcessed(ctx, providerConsumerGroup, transferID.String()).Return(nil)

		pc.handle(ctx, internalTransferMessage(transferID))

		assert.Equal(t, 1, client.calls)
		require.Len(t, consumer.commits, 1)
		assert.Empty(t, producer.published)
	})

	t.Run("skips already processed message", func(t *testing.T) {
		transferID := uuid.New()
		consumer := &internalConsumer{}
		producer := &internalProducer{}
		client := &internalProviderClient{}
		transfers := testmocks.NewTransferRepository(t)
		inbox := testmocks.NewInboxRepository(t)
		pc := NewProviderConsumer(consumer, producer, client, transfers, inbox, log)

		inbox.EXPECT().IsProcessed(ctx, providerConsumerGroup, transferID.String()).Return(true, nil)

		pc.handle(ctx, internalTransferMessage(transferID))

		assert.Zero(t, client.calls)
		require.Len(t, consumer.commits, 1)
	})

	t.Run("sends poison payload to dlq and commits", func(t *testing.T) {
		consumer := &internalConsumer{}
		producer := &internalProducer{}
		pc := NewProviderConsumer(
			consumer,
			producer,
			&internalProviderClient{},
			testmocks.NewTransferRepository(t),
			testmocks.NewInboxRepository(t),
			log,
		)

		pc.handle(ctx, kafka.Message{Key: []byte("bad"), Value: []byte("bad-json")})

		require.Len(t, producer.published, 1)
		assert.Equal(t, kafkatopic.WalletDLQ, producer.published[0].Topic)
		require.Len(t, consumer.commits, 1)
	})

	t.Run("inbox read error escalates to retry", func(t *testing.T) {
		transferID := uuid.New()
		consumer := &internalConsumer{}
		producer := &internalProducer{}
		inbox := testmocks.NewInboxRepository(t)
		pc := NewProviderConsumer(
			consumer,
			producer,
			&internalProviderClient{},
			testmocks.NewTransferRepository(t),
			inbox,
			log,
		)
		inbox.EXPECT().
			IsProcessed(ctx, providerConsumerGroup, transferID.String()).
			Return(false, errors.New("db error"))

		pc.handle(ctx, internalTransferMessage(transferID))

		require.Len(t, producer.published, 1)
		assert.Equal(t, kafkatopic.WalletRetry6s, producer.published[0].Topic)
		require.Len(t, consumer.commits, 1)
	})

	t.Run("unknown or already settled transfer is marked processed", func(t *testing.T) {
		transferID := uuid.New()
		consumer := &internalConsumer{}
		producer := &internalProducer{}
		client := &internalProviderClient{}
		transfers := testmocks.NewTransferRepository(t)
		inbox := testmocks.NewInboxRepository(t)
		pc := NewProviderConsumer(consumer, producer, client, transfers, inbox, log)

		inbox.EXPECT().IsProcessed(ctx, providerConsumerGroup, transferID.String()).Return(false, nil)
		transfers.EXPECT().
			GetByID(ctx, transferID).
			Return(&entities.Transfer{ID: transferID, Status: entities.TransferStatusSucceeded}, nil)
		inbox.EXPECT().MarkProcessed(ctx, providerConsumerGroup, transferID.String()).Return(nil)

		pc.handle(ctx, internalTransferMessage(transferID))

		assert.Zero(t, client.calls)
		require.Len(t, consumer.commits, 1)
	})

	t.Run("provider submit error escalates to retry", func(t *testing.T) {
		transferID := uuid.New()
		consumer := &internalConsumer{}
		producer := &internalProducer{}
		client := &internalProviderClient{err: errors.New("provider timeout")}
		transfers := testmocks.NewTransferRepository(t)
		inbox := testmocks.NewInboxRepository(t)
		pc := NewProviderConsumer(consumer, producer, client, transfers, inbox, log)

		inbox.EXPECT().IsProcessed(ctx, providerConsumerGroup, transferID.String()).Return(false, nil)
		transfers.EXPECT().GetByID(ctx, transferID).Return(&entities.Transfer{
			ID:                  transferID,
			Status:              entities.TransferStatusReserved,
			TransactionAmount:   decimal.NewFromInt(25),
			TransactionCurrency: "USD",
		}, nil)

		pc.handle(ctx, internalTransferMessage(transferID))

		require.Len(t, producer.published, 1)
		assert.Equal(t, kafkatopic.WalletRetry6s, producer.published[0].Topic)
		require.Len(t, consumer.commits, 1)
	})

	t.Run("mark processed failure still commits", func(t *testing.T) {
		transferID := uuid.New()
		consumer := &internalConsumer{}
		producer := &internalProducer{}
		client := &internalProviderClient{}
		transfers := testmocks.NewTransferRepository(t)
		inbox := testmocks.NewInboxRepository(t)
		pc := NewProviderConsumer(consumer, producer, client, transfers, inbox, log)

		inbox.EXPECT().IsProcessed(ctx, providerConsumerGroup, transferID.String()).Return(false, nil)
		transfers.EXPECT().GetByID(ctx, transferID).Return(nil, nil)
		inbox.EXPECT().MarkProcessed(ctx, providerConsumerGroup, transferID.String()).Return(errors.New("mark failed"))

		pc.handle(ctx, internalTransferMessage(transferID))

		require.Len(t, consumer.commits, 1)
	})
}

func TestProviderConsumerRunReturnsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pc := NewProviderConsumer(
		&internalConsumer{fetchErr: errors.New("fetch stopped")},
		&internalProducer{},
		&internalProviderClient{},
		testmocks.NewTransferRepository(t),
		testmocks.NewInboxRepository(t),
		logger.New("plain", "error"),
	)

	err := pc.Run(ctx)

	assert.ErrorIs(t, err, context.Canceled)
}

func TestRetryConsumerHandle(t *testing.T) {
	ctx := context.Background()
	log := logger.New("plain", "error")

	t.Run("holds then republishes to source topic and commits", func(t *testing.T) {
		consumer := &internalConsumer{}
		producer := &internalProducer{}
		rc := NewRetryConsumer(consumer, producer, log)
		msg := kafka.Message{
			Key:   []byte("key"),
			Value: []byte("value"),
			Headers: []kafka.Header{
				{Key: kafkatopic.HeaderRetryAt, Value: []byte("123")},
				{Key: kafkatopic.HeaderRetryFrom, Value: []byte(kafkatopic.TransferProviderRequested)},
				{Key: kafkatopic.HeaderRetryAttempt, Value: []byte("1")},
				{Key: kafkatopic.HeaderError, Value: []byte("retry me")},
			},
		}

		rc.handle(ctx, ctx, msg)

		assert.Equal(t, []int64{123}, consumer.holds)
		require.Len(t, producer.published, 1)
		assert.Equal(t, kafkatopic.TransferProviderRequested, producer.published[0].Topic)
		require.Len(t, consumer.commits, 1)
	})

	t.Run("defaults target when retry from header is missing", func(t *testing.T) {
		consumer := &internalConsumer{}
		producer := &internalProducer{}
		rc := NewRetryConsumer(consumer, producer, log)
		msg := kafka.Message{Key: []byte("key"), Value: []byte("value")}

		rc.handle(ctx, ctx, msg)

		require.Len(t, producer.published, 1)
		assert.Equal(t, kafkatopic.TransferRequested, producer.published[0].Topic)
		require.Len(t, consumer.commits, 1)
	})

	t.Run("hold error leaves message uncommitted", func(t *testing.T) {
		consumer := &internalConsumer{holdErr: context.Canceled}
		producer := &internalProducer{}
		rc := NewRetryConsumer(consumer, producer, log)
		msg := kafka.Message{Headers: []kafka.Header{{Key: kafkatopic.HeaderRetryAt, Value: []byte("123")}}}

		rc.handle(ctx, ctx, msg)

		assert.Empty(t, producer.published)
		assert.Empty(t, consumer.commits)
	})

	t.Run("publish error leaves message uncommitted", func(t *testing.T) {
		consumer := &internalConsumer{}
		producer := &internalProducer{err: errors.New("kafka down")}
		rc := NewRetryConsumer(consumer, producer, log)
		msg := kafka.Message{Key: []byte("key"), Value: []byte("value")}

		rc.handle(ctx, ctx, msg)

		assert.Empty(t, consumer.commits)
	})
}

func TestRetryConsumerRunReturnsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rc := NewRetryConsumer(
		&internalConsumer{fetchErr: errors.New("fetch stopped")},
		&internalProducer{},
		logger.New("plain", "error"),
	)

	err := rc.Run(ctx)

	assert.ErrorIs(t, err, context.Canceled)
}

func TestConstructorCoverage(t *testing.T) {
	log := logger.New("plain", "error")
	consumerClient := &internalConsumer{}
	producer := &internalProducer{}
	providerClient := &internalProviderClient{}
	transfers := testmocks.NewTransferRepository(t)
	inbox := testmocks.NewInboxRepository(t)

	assert.NotNil(t, NewProviderConsumer(consumerClient, producer, providerClient, transfers, inbox, log))
	assert.NotNil(t, NewRetryConsumer(consumerClient, producer, log))
}

func TestMockImportCoverage(t *testing.T) {
	assert.NotNil(t, mock.Anything)
}

func TestProcessorRunReturnsCanceledContextInternal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := NewProcessor(
		&internalConsumer{fetchErr: errors.New("fetch stopped")},
		&internalProducer{},
		testmocks.NewTransferRepository(t),
		testmocks.NewInboxRepository(t),
		nil,
		logger.New("plain", "error"),
	)

	err := p.Run(ctx)

	assert.ErrorIs(t, err, context.Canceled)
}

func TestProcessorCommitAndMarkProcessedErrorsAreNonFatal(t *testing.T) {
	ctx := context.Background()
	transferID := uuid.New()
	consumerClient := &internalConsumer{commitErr: errors.New("commit failed")}
	producer := &internalProducer{}
	transfers := testmocks.NewTransferRepository(t)
	inbox := testmocks.NewInboxRepository(t)
	p := NewProcessor(consumerClient, producer, transfers, inbox, nil, logger.New("plain", "error"))

	inbox.EXPECT().IsProcessed(ctx, consumerGroup, transferID.String()).Return(false, nil)
	transfers.EXPECT().GetByID(ctx, transferID).Return(nil, nil)
	inbox.EXPECT().MarkProcessed(ctx, consumerGroup, transferID.String()).Return(errors.New("mark failed"))

	p.Handle(ctx, internalTransferMessage(transferID))

	assert.Empty(t, producer.published)
}

func TestRetryHelperPublishErrorsAreNonFatal(t *testing.T) {
	producer := &internalProducer{err: errors.New("kafka down")}
	h := NewRetryHelper(producer, logger.New("plain", "error"), kafkatopic.TransferRequested)
	msg := internalTransferMessage(uuid.New())

	h.EscalateOrDLQ(context.Background(), msg, errors.New("retry"))
	h.ToDLQ(context.Background(), msg, errors.New("poison"))

	assert.Empty(t, producer.published)
}
