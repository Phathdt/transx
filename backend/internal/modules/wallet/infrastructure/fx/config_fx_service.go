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
	rates map[string]decimal.Decimal
}

var _ interfaces.FXService = (*ConfigService)(nil)

func NewConfigService(cfg config.FX) *ConfigService {
	// Parse and validate rates once at construction so each Quote is a map
	// lookup; malformed or non-positive rates are dropped and surface as an
	// unavailable corridor at quote time.
	rates := make(map[string]decimal.Decimal, len(cfg.Rates))
	for k, v := range cfg.Rates {
		rate, err := decimal.NewFromString(strings.TrimSpace(v))
		if err != nil || rate.LessThanOrEqual(decimal.Zero) {
			continue
		}
		rates[normalizeRateKey(k)] = rate
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
	rate, ok := s.rates[key]
	if !ok {
		return interfaces.FXQuote{}, fmt.Errorf("%w: %s", interfaces.ErrFXRateUnavailable, key)
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
