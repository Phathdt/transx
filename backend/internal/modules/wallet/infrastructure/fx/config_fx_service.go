package fx

import (
	"context"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"transx/internal/modules/wallet/domain/interfaces"
	"transx/internal/platform/config"
)

const quoteSourceConfig = "config"

// ConfigService quotes FX from static config rates. Same-currency corridors are
// always available at rate 1; cross-currency corridors require a FROM_TO key.
type ConfigService struct {
	rates map[string]string
}

var _ interfaces.FXService = (*ConfigService)(nil)

func NewConfigService(cfg config.FX) *ConfigService {
	rates := make(map[string]string, len(cfg.Rates))
	for k, v := range cfg.Rates {
		rates[normalizeRateKey(k)] = strings.TrimSpace(v)
	}
	return &ConfigService{rates: rates}
}

func (s *ConfigService) Quote(
	_ context.Context,
	amount decimal.Decimal,
	fromCurrency, toCurrency string,
) (interfaces.FXQuote, error) {
	from := normalizeCurrency(fromCurrency)
	to := normalizeCurrency(toCurrency)
	if from == "" || to == "" {
		return interfaces.FXQuote{}, fmt.Errorf("%w: empty currency", interfaces.ErrFXRateUnavailable)
	}
	if from == to {
		return interfaces.FXQuote{
			Amount:   roundByCurrency(amount, to),
			Currency: to,
			Rate:     decimal.NewFromInt(1),
			Source:   quoteSourceConfig,
		}, nil
	}

	key := from + "_" + to
	rateText, ok := s.rates[key]
	if !ok || rateText == "" {
		return interfaces.FXQuote{}, fmt.Errorf("%w: %s", interfaces.ErrFXRateUnavailable, key)
	}
	rate, err := decimal.NewFromString(rateText)
	if err != nil || rate.LessThanOrEqual(decimal.Zero) {
		return interfaces.FXQuote{}, fmt.Errorf("invalid fx rate %s: %w", key, interfaces.ErrFXRateUnavailable)
	}
	return interfaces.FXQuote{
		Amount:   roundByCurrency(amount.Mul(rate), to),
		Currency: to,
		Rate:     rate,
		Source:   quoteSourceConfig,
	}, nil
}

func normalizeCurrency(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func normalizeRateKey(key string) string {
	parts := strings.Split(normalizeCurrency(key), "_")
	if len(parts) != 2 {
		return normalizeCurrency(key)
	}
	return parts[0] + "_" + parts[1]
}

func roundByCurrency(amount decimal.Decimal, currency string) decimal.Decimal {
	return amount.Round(currencyScale(currency))
}

func currencyScale(currency string) int32 {
	switch normalizeCurrency(currency) {
	case "VND":
		return 0
	case "USD", "EUR":
		return 2
	default:
		return 4
	}
}
