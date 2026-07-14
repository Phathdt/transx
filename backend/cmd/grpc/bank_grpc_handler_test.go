package grpc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"transx/internal/common/provider"
	bankv1 "transx/internal/platform/grpc/gen/bank/v1"
)

func TestBankServerSubmit(t *testing.T) {
	ctx := context.Background()
	transferID := uuid.New()

	t.Run("always_success returns SUCCESS with a reference id", func(t *testing.T) {
		server := NewBankServer(provider.ModeAlwaysSuccess)
		resp, err := server.Submit(ctx, &bankv1.SubmitRequest{
			TransferId: transferID.String(),
			Amount:     "100",
			Currency:   "USD",
		})

		require.NoError(t, err)
		assert.Equal(t, "SUCCESS", resp.GetOutcome())
		assert.NotEmpty(t, resp.GetReferenceId())
		assert.Empty(t, resp.GetReason())
	})

	t.Run("always_failure returns FAILURE with a reason", func(t *testing.T) {
		server := NewBankServer(provider.ModeAlwaysFailure)
		resp, err := server.Submit(ctx, &bankv1.SubmitRequest{
			TransferId: transferID.String(),
			Amount:     "100",
			Currency:   "USD",
		})

		require.NoError(t, err)
		assert.Equal(t, "FAILURE", resp.GetOutcome())
		assert.NotEmpty(t, resp.GetReason())
	})

	t.Run("always_timeout returns a transient gRPC error", func(t *testing.T) {
		server := NewBankServer(provider.ModeAlwaysTimeout)
		_, err := server.Submit(ctx, &bankv1.SubmitRequest{
			TransferId: transferID.String(),
			Amount:     "100",
			Currency:   "USD",
		})

		assert.Equal(t, codes.DeadlineExceeded, status.Code(err))
	})

	t.Run("invalid transfer_id returns InvalidArgument", func(t *testing.T) {
		server := NewBankServer(provider.ModeAlwaysSuccess)
		_, err := server.Submit(ctx, &bankv1.SubmitRequest{TransferId: "not-a-uuid", Amount: "100"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("invalid amount returns InvalidArgument", func(t *testing.T) {
		server := NewBankServer(provider.ModeAlwaysSuccess)
		_, err := server.Submit(ctx, &bankv1.SubmitRequest{
			TransferId: transferID.String(),
			Amount:     "not-a-number",
		})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}

func TestBankServerQuery(t *testing.T) {
	ctx := context.Background()
	transferID := uuid.New()

	t.Run("derives the same outcome as Submit for the configured mode", func(t *testing.T) {
		server := NewBankServer(provider.ModeAlwaysSuccess)
		resp, err := server.Query(ctx, &bankv1.QueryRequest{TransferId: transferID.String()})

		require.NoError(t, err)
		assert.Equal(t, "SUCCESS", resp.GetOutcome())
	})

	t.Run("stateless: an unknown transfer_id still resolves from mode", func(t *testing.T) {
		server := NewBankServer(provider.ModeAlwaysFailure)
		resp, err := server.Query(ctx, &bankv1.QueryRequest{TransferId: uuid.New().String()})

		require.NoError(t, err)
		assert.Equal(t, "FAILURE", resp.GetOutcome())
	})

	t.Run("invalid transfer_id returns InvalidArgument", func(t *testing.T) {
		server := NewBankServer(provider.ModeAlwaysSuccess)
		_, err := server.Query(ctx, &bankv1.QueryRequest{TransferId: "not-a-uuid"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}

func TestBankServerRandomMode(t *testing.T) {
	ctx := context.Background()
	transferID := uuid.New()
	server := NewBankServer(provider.ModeRandom)

	submit, err := server.Submit(ctx, &bankv1.SubmitRequest{
		TransferId: transferID.String(),
		Amount:     "100",
		Currency:   "USD",
	})
	require.NoError(t, err)
	query, err := server.Query(ctx, &bankv1.QueryRequest{TransferId: transferID.String()})
	require.NoError(t, err)
	assert.Equal(t, submit.GetOutcome(), query.GetOutcome())
	assert.Equal(t, submit.GetReferenceId(), query.GetReferenceId())
	assert.Equal(t, submit.GetReason(), query.GetReason())
	assert.Contains(t, []string{"SUCCESS", "FAILURE"}, submit.GetOutcome())
}
