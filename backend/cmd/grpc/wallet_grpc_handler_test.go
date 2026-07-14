package grpc

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"transx/internal/modules/wallet/domain/interfaces"
	walletv1 "transx/internal/platform/grpc/gen/wallet/v1"
	"transx/internal/testmocks"
)

// errBoom is an unmapped repository error used to exercise the default
// codes.Internal branch of moneyRepositoryError.
var errBoom = errors.New("boom")

func TestWalletServerMove(t *testing.T) {
	ctx := context.Background()
	transferID := uuid.New()

	t.Run("success returns both balances", func(t *testing.T) {
		money := testmocks.NewMoneyRepository(t)
		money.EXPECT().
			Move(ctx, transferID, "MOVE", interfaces.MoveInput{
				FromAccountRef:      "ACC-1",
				ToAccountRef:        "ACC-2",
				SourceAmount:        decimal.RequireFromString("100"),
				SourceCurrency:      "USD",
				DestinationAmount:   decimal.RequireFromString("92"),
				DestinationCurrency: "EUR",
				FeeAmount:           decimal.RequireFromString("1"),
				FeeCurrency:         "USD",
			}).
			Return(interfaces.MoveResult{
				FromAvailableBalance: decimal.RequireFromString("899"),
				ToAvailableBalance:   decimal.RequireFromString("192"),
			}, nil)

		server := NewWalletServer(money)
		resp, err := server.Move(ctx, &walletv1.MoveRequest{
			TransferId:          transferID.String(),
			Operation:           "MOVE",
			FromAccountRef:      "ACC-1",
			ToAccountRef:        "ACC-2",
			SourceAmount:        "100",
			SourceCurrency:      "USD",
			DestinationAmount:   "92",
			DestinationCurrency: "EUR",
			FeeAmount:           "1",
			FeeCurrency:         "USD",
		})

		require.NoError(t, err)
		assert.Equal(t, "899", resp.GetFromAvailableBalance())
		assert.Equal(t, "192", resp.GetToAvailableBalance())
	})

	t.Run("repeat with same transfer_id and operation is idempotent at the repository", func(t *testing.T) {
		money := testmocks.NewMoneyRepository(t)
		money.EXPECT().
			Move(ctx, transferID, "MOVE", interfaces.MoveInput{
				FromAccountRef:      "ACC-1",
				ToAccountRef:        "ACC-2",
				SourceAmount:        decimal.RequireFromString("100"),
				SourceCurrency:      "USD",
				DestinationAmount:   decimal.RequireFromString("92"),
				DestinationCurrency: "EUR",
				FeeAmount:           decimal.Zero,
			}).
			Return(interfaces.MoveResult{
				FromAvailableBalance: decimal.RequireFromString("899"),
				ToAvailableBalance:   decimal.RequireFromString("192"),
			}, nil).
			Twice()

		server := NewWalletServer(money)
		req := &walletv1.MoveRequest{
			TransferId:          transferID.String(),
			Operation:           "MOVE",
			FromAccountRef:      "ACC-1",
			ToAccountRef:        "ACC-2",
			SourceAmount:        "100",
			SourceCurrency:      "USD",
			DestinationAmount:   "92",
			DestinationCurrency: "EUR",
		}

		resp1, err1 := server.Move(ctx, req)
		resp2, err2 := server.Move(ctx, req)

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.Equal(t, resp1.GetFromAvailableBalance(), resp2.GetFromAvailableBalance())
		assert.Equal(t, resp1.GetToAvailableBalance(), resp2.GetToAvailableBalance())
	})

	t.Run("invalid transfer_id returns InvalidArgument", func(t *testing.T) {
		server := NewWalletServer(testmocks.NewMoneyRepository(t))
		_, err := server.Move(ctx, &walletv1.MoveRequest{TransferId: "not-a-uuid", Operation: "MOVE"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("missing operation returns InvalidArgument", func(t *testing.T) {
		server := NewWalletServer(testmocks.NewMoneyRepository(t))
		_, err := server.Move(ctx, &walletv1.MoveRequest{TransferId: transferID.String()})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("invalid source_amount returns InvalidArgument", func(t *testing.T) {
		server := NewWalletServer(testmocks.NewMoneyRepository(t))
		_, err := server.Move(ctx, &walletv1.MoveRequest{
			TransferId:   transferID.String(),
			Operation:    "MOVE",
			SourceAmount: "not-a-number",
		})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("insufficient funds maps to FailedPrecondition", func(t *testing.T) {
		money := testmocks.NewMoneyRepository(t)
		money.EXPECT().
			Move(ctx, transferID, "MOVE", interfaces.MoveInput{
				FromAccountRef:      "ACC-1",
				ToAccountRef:        "ACC-2",
				SourceAmount:        decimal.RequireFromString("100"),
				SourceCurrency:      "USD",
				DestinationAmount:   decimal.RequireFromString("92"),
				DestinationCurrency: "EUR",
				FeeAmount:           decimal.Zero,
			}).
			Return(interfaces.MoveResult{}, interfaces.ErrInsufficientFunds)

		server := NewWalletServer(money)
		_, err := server.Move(ctx, &walletv1.MoveRequest{
			TransferId:          transferID.String(),
			Operation:           "MOVE",
			FromAccountRef:      "ACC-1",
			ToAccountRef:        "ACC-2",
			SourceAmount:        "100",
			SourceCurrency:      "USD",
			DestinationAmount:   "92",
			DestinationCurrency: "EUR",
		})

		assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	})
}

func TestWalletServerHold(t *testing.T) {
	ctx := context.Background()
	transferID := uuid.New()

	t.Run("success returns available and hold balance", func(t *testing.T) {
		money := testmocks.NewMoneyRepository(t)
		money.EXPECT().
			Hold(ctx, transferID, "HOLD", "ACC-1", decimal.RequireFromString("50"), "USD").
			Return(interfaces.HoldResult{
				AvailableBalance: decimal.RequireFromString("50"),
				HoldBalance:      decimal.RequireFromString("50"),
			}, nil)

		server := NewWalletServer(money)
		resp, err := server.Hold(ctx, &walletv1.HoldRequest{
			TransferId: transferID.String(),
			Operation:  "HOLD",
			AccountRef: "ACC-1",
			Amount:     "50",
			Currency:   "USD",
		})

		require.NoError(t, err)
		assert.Equal(t, "50", resp.GetAvailableBalance())
		assert.Equal(t, "50", resp.GetHoldBalance())
	})

	t.Run("account not found maps to NotFound", func(t *testing.T) {
		money := testmocks.NewMoneyRepository(t)
		money.EXPECT().
			Hold(ctx, transferID, "HOLD", "ACC-missing", decimal.RequireFromString("50"), "USD").
			Return(interfaces.HoldResult{}, interfaces.ErrAccountNotFound)

		server := NewWalletServer(money)
		_, err := server.Hold(ctx, &walletv1.HoldRequest{
			TransferId: transferID.String(),
			Operation:  "HOLD",
			AccountRef: "ACC-missing",
			Amount:     "50",
			Currency:   "USD",
		})

		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("invalid amount returns InvalidArgument", func(t *testing.T) {
		server := NewWalletServer(testmocks.NewMoneyRepository(t))
		_, err := server.Hold(ctx, &walletv1.HoldRequest{
			TransferId: transferID.String(),
			Operation:  "HOLD",
			AccountRef: "ACC-1",
			Amount:     "not-a-number",
		})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}

func TestWalletServerSettleHold(t *testing.T) {
	ctx := context.Background()
	transferID := uuid.New()

	t.Run("success returns updated balances", func(t *testing.T) {
		money := testmocks.NewMoneyRepository(t)
		money.EXPECT().
			SettleHold(ctx, transferID, "SETTLE_HOLD", "ACC-1", decimal.RequireFromString("50"), "USD").
			Return(interfaces.HoldResult{
				AvailableBalance: decimal.RequireFromString("100"),
				HoldBalance:      decimal.Zero,
			}, nil)

		server := NewWalletServer(money)
		resp, err := server.SettleHold(ctx, &walletv1.SettleHoldRequest{
			TransferId: transferID.String(),
			Operation:  "SETTLE_HOLD",
			AccountRef: "ACC-1",
			Amount:     "50",
			Currency:   "USD",
		})

		require.NoError(t, err)
		assert.Equal(t, "100", resp.GetAvailableBalance())
		assert.Equal(t, "0", resp.GetHoldBalance())
	})

	t.Run("invalid transfer_id returns InvalidArgument", func(t *testing.T) {
		server := NewWalletServer(testmocks.NewMoneyRepository(t))
		_, err := server.SettleHold(
			ctx,
			&walletv1.SettleHoldRequest{TransferId: "not-a-uuid", Operation: "SETTLE_HOLD"},
		)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("currency mismatch maps to FailedPrecondition", func(t *testing.T) {
		money := testmocks.NewMoneyRepository(t)
		money.EXPECT().
			SettleHold(ctx, transferID, "SETTLE_HOLD", "ACC-1", decimal.RequireFromString("50"), "USD").
			Return(interfaces.HoldResult{}, interfaces.ErrCurrencyMismatch)

		server := NewWalletServer(money)
		_, err := server.SettleHold(ctx, &walletv1.SettleHoldRequest{
			TransferId: transferID.String(),
			Operation:  "SETTLE_HOLD",
			AccountRef: "ACC-1",
			Amount:     "50",
			Currency:   "USD",
		})

		assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	})

	t.Run("unmapped repository error becomes Internal", func(t *testing.T) {
		money := testmocks.NewMoneyRepository(t)
		money.EXPECT().
			SettleHold(ctx, transferID, "SETTLE_HOLD", "ACC-1", decimal.RequireFromString("50"), "USD").
			Return(interfaces.HoldResult{}, errBoom)

		server := NewWalletServer(money)
		_, err := server.SettleHold(ctx, &walletv1.SettleHoldRequest{
			TransferId: transferID.String(),
			Operation:  "SETTLE_HOLD",
			AccountRef: "ACC-1",
			Amount:     "50",
			Currency:   "USD",
		})

		assert.Equal(t, codes.Internal, status.Code(err))
	})
}

func TestWalletServerReleaseHold(t *testing.T) {
	ctx := context.Background()
	transferID := uuid.New()

	t.Run("success returns updated balances", func(t *testing.T) {
		money := testmocks.NewMoneyRepository(t)
		money.EXPECT().
			ReleaseHold(ctx, transferID, "RELEASE_HOLD", "ACC-1", decimal.RequireFromString("50"), "USD").
			Return(interfaces.HoldResult{
				AvailableBalance: decimal.RequireFromString("150"),
				HoldBalance:      decimal.Zero,
			}, nil)

		server := NewWalletServer(money)
		resp, err := server.ReleaseHold(ctx, &walletv1.ReleaseHoldRequest{
			TransferId: transferID.String(),
			Operation:  "RELEASE_HOLD",
			AccountRef: "ACC-1",
			Amount:     "50",
			Currency:   "USD",
		})

		require.NoError(t, err)
		assert.Equal(t, "150", resp.GetAvailableBalance())
		assert.Equal(t, "0", resp.GetHoldBalance())
	})

	t.Run("missing operation returns InvalidArgument", func(t *testing.T) {
		server := NewWalletServer(testmocks.NewMoneyRepository(t))
		_, err := server.ReleaseHold(ctx, &walletv1.ReleaseHoldRequest{TransferId: transferID.String()})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("invalid amount returns InvalidArgument", func(t *testing.T) {
		server := NewWalletServer(testmocks.NewMoneyRepository(t))
		_, err := server.ReleaseHold(ctx, &walletv1.ReleaseHoldRequest{
			TransferId: transferID.String(),
			Operation:  "RELEASE_HOLD",
			AccountRef: "ACC-1",
			Amount:     "not-a-number",
		})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}
