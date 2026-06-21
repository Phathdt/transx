package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"transx/internal/modules/fx/application/services"
	"transx/internal/platform/config"
	fxv1 "transx/internal/platform/grpc/gen/fx/v1"
)

func newTestFXServer() *FXServer {
	return NewFXServer(services.NewConfigService(config.FX{
		Rates: map[string]string{"VND_USD": "0.00003924"},
		Fees:  map[string]string{"VND": "10000"},
	}))
}

func TestFXServerQuote(t *testing.T) {
	ctx := context.Background()
	server := newTestFXServer()

	t.Run("cross-currency returns rounded amount and rate", func(t *testing.T) {
		resp, err := server.Quote(ctx, &fxv1.QuoteRequest{
			Amount:       "500000",
			FromCurrency: "VND",
			ToCurrency:   "USD",
		})

		require.NoError(t, err)
		assert.Equal(t, "19.62", resp.GetAmount())
		assert.Equal(t, "USD", resp.GetCurrency())
		assert.Equal(t, "0.00003924", resp.GetRate())
		assert.Equal(t, "config", resp.GetSource())
	})

	t.Run("invalid amount returns InvalidArgument", func(t *testing.T) {
		_, err := server.Quote(ctx, &fxv1.QuoteRequest{
			Amount:       "not-a-number",
			FromCurrency: "VND",
			ToCurrency:   "USD",
		})

		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("unconfigured corridor returns FailedPrecondition", func(t *testing.T) {
		_, err := server.Quote(ctx, &fxv1.QuoteRequest{
			Amount:       "1",
			FromCurrency: "USD",
			ToCurrency:   "GBP",
		})

		assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	})
}

func TestFXServerQuoteFee(t *testing.T) {
	ctx := context.Background()
	server := newTestFXServer()

	t.Run("cross-currency returns configured flat fee", func(t *testing.T) {
		resp, err := server.QuoteFee(ctx, &fxv1.QuoteFeeRequest{
			TransactionCurrency: "USD",
			SourceCurrency:      "VND",
		})

		require.NoError(t, err)
		assert.Equal(t, "10000", resp.GetAmount())
		assert.Equal(t, "VND", resp.GetCurrency())
	})

	t.Run("same currency returns zero fee", func(t *testing.T) {
		resp, err := server.QuoteFee(ctx, &fxv1.QuoteFeeRequest{
			TransactionCurrency: "VND",
			SourceCurrency:      "VND",
		})

		require.NoError(t, err)
		assert.Equal(t, "0", resp.GetAmount())
		assert.Equal(t, "VND", resp.GetCurrency())
	})
}
