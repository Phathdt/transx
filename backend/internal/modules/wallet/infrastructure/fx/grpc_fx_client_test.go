package fx_test

import (
	"context"
	"net"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	cmdgrpc "transx/cmd/grpc"
	fxservices "transx/internal/modules/fx/application/services"
	"transx/internal/modules/wallet/domain/interfaces"
	walletfx "transx/internal/modules/wallet/infrastructure/fx"
	"transx/internal/platform/config"
	fxv1 "transx/internal/platform/grpc/gen/fx/v1"
)

func newTestClient(t *testing.T) *walletfx.GRPCClient {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	fxv1.RegisterFXServiceServer(server, cmdgrpc.NewFXServer(fxservices.NewConfigService(config.FX{
		Rates: map[string]string{"VND_USD": "0.00003924"},
		Fees:  map[string]string{"VND": "10000"},
	})))
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return walletfx.NewGRPCClient(fxv1.NewFXServiceClient(conn))
}

func TestGRPCClientQuote(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)

	t.Run("maps successful quote", func(t *testing.T) {
		quote, err := client.Quote(ctx, decimal.RequireFromString("500000"), "VND", "USD")

		require.NoError(t, err)
		assert.Equal(t, "19.62", quote.Amount.String())
		assert.Equal(t, "USD", quote.Currency)
		assert.Equal(t, "0.00003924", quote.Rate.String())
		assert.Equal(t, "config", quote.Source)
	})

	t.Run("unconfigured corridor maps to ErrFXRateUnavailable", func(t *testing.T) {
		_, err := client.Quote(ctx, decimal.NewFromInt(1), "USD", "GBP")

		assert.ErrorIs(t, err, interfaces.ErrFXRateUnavailable)
	})
}

func TestGRPCClientQuoteFee(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)

	t.Run("maps configured flat fee", func(t *testing.T) {
		fee, err := client.QuoteFee(ctx, "USD", "VND")

		require.NoError(t, err)
		assert.Equal(t, "10000", fee.Amount.String())
		assert.Equal(t, "VND", fee.Currency)
	})

	t.Run("same currency returns zero fee", func(t *testing.T) {
		fee, err := client.QuoteFee(ctx, "VND", "VND")

		require.NoError(t, err)
		assert.Equal(t, "0", fee.Amount.String())
		assert.Equal(t, "VND", fee.Currency)
	})
}

// stubFXClient is a hand-written fxv1.FXServiceClient that returns canned
// responses/errors so the adapter's error and parse paths can be exercised
// without a live server.
type stubFXClient struct {
	quoteResp *fxv1.QuoteResponse
	quoteErr  error
	feeResp   *fxv1.QuoteFeeResponse
	feeErr    error
}

func (s stubFXClient) Quote(
	_ context.Context,
	_ *fxv1.QuoteRequest,
	_ ...grpc.CallOption,
) (*fxv1.QuoteResponse, error) {
	return s.quoteResp, s.quoteErr
}

func (s stubFXClient) QuoteFee(
	_ context.Context,
	_ *fxv1.QuoteFeeRequest,
	_ ...grpc.CallOption,
) (*fxv1.QuoteFeeResponse, error) {
	return s.feeResp, s.feeErr
}

func TestGRPCClientQuoteErrorPaths(t *testing.T) {
	ctx := context.Background()

	t.Run("non-FailedPrecondition error is propagated", func(t *testing.T) {
		client := walletfx.NewGRPCClient(stubFXClient{quoteErr: status.Error(codes.Unavailable, "down")})
		_, err := client.Quote(ctx, decimal.NewFromInt(1), "VND", "USD")

		require.Error(t, err)
		assert.Equal(t, codes.Unavailable, status.Code(err))
	})

	t.Run("malformed amount returns parse error", func(t *testing.T) {
		client := walletfx.NewGRPCClient(stubFXClient{
			quoteResp: &fxv1.QuoteResponse{Amount: "bad", Currency: "USD", Rate: "1"},
		})
		_, err := client.Quote(ctx, decimal.NewFromInt(1), "VND", "USD")

		require.Error(t, err)
	})

	t.Run("malformed rate returns parse error", func(t *testing.T) {
		client := walletfx.NewGRPCClient(stubFXClient{
			quoteResp: &fxv1.QuoteResponse{Amount: "1", Currency: "USD", Rate: "bad"},
		})
		_, err := client.Quote(ctx, decimal.NewFromInt(1), "VND", "USD")

		require.Error(t, err)
	})
}

func TestGRPCClientQuoteFeeErrorPaths(t *testing.T) {
	ctx := context.Background()

	t.Run("non-FailedPrecondition error is propagated", func(t *testing.T) {
		client := walletfx.NewGRPCClient(stubFXClient{feeErr: status.Error(codes.Unavailable, "down")})
		_, err := client.QuoteFee(ctx, "USD", "VND")

		require.Error(t, err)
		assert.Equal(t, codes.Unavailable, status.Code(err))
	})

	t.Run("malformed amount returns parse error", func(t *testing.T) {
		client := walletfx.NewGRPCClient(stubFXClient{
			feeResp: &fxv1.QuoteFeeResponse{Amount: "bad", Currency: "VND"},
		})
		_, err := client.QuoteFee(ctx, "USD", "VND")

		require.Error(t, err)
	})

	t.Run("FailedPrecondition maps to ErrFXRateUnavailable", func(t *testing.T) {
		client := walletfx.NewGRPCClient(stubFXClient{feeErr: status.Error(codes.FailedPrecondition, "no corridor")})
		_, err := client.QuoteFee(ctx, "USD", "VND")

		assert.ErrorIs(t, err, interfaces.ErrFXRateUnavailable)
	})
}
