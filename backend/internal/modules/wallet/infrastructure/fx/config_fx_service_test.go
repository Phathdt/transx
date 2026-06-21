package fx

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"transx/internal/modules/wallet/domain/interfaces"
	"transx/internal/platform/config"
)

func TestConfigServiceQuote(t *testing.T) {
	ctx := context.Background()
	service := NewConfigService(config.FX{Rates: map[string]string{
		" vnd_usd ": "0.00003924",
		"USD_VND":   "25484.20",
		"USD_EUR":   "0.925",
		"USDGBP":    "2",
		"EUR_USD":   "bad-rate",
		"EUR_VND":   "0",
	}})

	t.Run("identity quote normalizes and rounds by target currency", func(t *testing.T) {
		quote, err := service.Quote(ctx, decimal.RequireFromString("10.555"), " usd ", "USD")

		require.NoError(t, err)
		assert.Equal(t, "10.56", quote.Amount.String())
		assert.Equal(t, "USD", quote.Currency)
		assert.Equal(t, "1", quote.Rate.String())
		assert.Equal(t, quoteSourceConfig, quote.Source)
	})

	t.Run("cross currency uses configured direction and half up target scale", func(t *testing.T) {
		quote, err := service.Quote(ctx, decimal.RequireFromString("500000"), "VND", "USD")

		require.NoError(t, err)
		assert.Equal(t, "19.62", quote.Amount.String())
		assert.Equal(t, "USD", quote.Currency)
		assert.Equal(t, "0.00003924", quote.Rate.String())
	})

	t.Run("vnd target rounds to zero decimals", func(t *testing.T) {
		quote, err := service.Quote(ctx, decimal.RequireFromString("2.5"), "USD", "VND")

		require.NoError(t, err)
		assert.Equal(t, "63711", quote.Amount.String())
		assert.Equal(t, "VND", quote.Currency)
	})

	t.Run("unknown currency scale falls back to four decimals", func(t *testing.T) {
		quote, err := service.Quote(ctx, decimal.RequireFromString("1.23456"), "XXX", "XXX")

		require.NoError(t, err)
		assert.Equal(t, "1.2346", quote.Amount.String())
	})

	t.Run("missing malformed-key and empty currencies return recognizable unavailable error", func(t *testing.T) {
		_, err := service.Quote(ctx, decimal.NewFromInt(1), "USD", "GBP")
		assert.ErrorIs(t, err, interfaces.ErrFXRateUnavailable)

		_, err = service.Quote(ctx, decimal.NewFromInt(1), "USDGBP", "EUR")
		assert.ErrorIs(t, err, interfaces.ErrFXRateUnavailable)

		_, err = service.Quote(ctx, decimal.NewFromInt(1), "", "USD")
		assert.ErrorIs(t, err, interfaces.ErrFXRateUnavailable)
	})

	t.Run("invalid and non-positive configured rates return unavailable error", func(t *testing.T) {
		_, err := service.Quote(ctx, decimal.NewFromInt(1), "EUR", "USD")
		assert.True(t, errors.Is(err, interfaces.ErrFXRateUnavailable))

		_, err = service.Quote(ctx, decimal.NewFromInt(1), "EUR", "VND")
		assert.True(t, errors.Is(err, interfaces.ErrFXRateUnavailable))
	})
}
