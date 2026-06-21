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
// Cross-currency transfers also carry a flat fee keyed by the source currency.
type ConfigService struct {
	rates map[string]decimal.Decimal
	fees  map[string]decimal.Decimal
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
	// Flat fee per source currency. A missing or non-positive entry means no
	// fee for that currency.
	fees := make(map[string]decimal.Decimal, len(cfg.Fees))
	for k, v := range cfg.Fees {
		fee, err := decimal.NewFromString(strings.TrimSpace(v))
		if err != nil || fee.LessThanOrEqual(decimal.Zero) {
			continue
		}
		fees[normalizeCurrency(k)] = fee
	}
	return &ConfigService{rates: rates, fees: fees}
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

func (s *ConfigService) QuoteFee(
	_ context.Context,
	transactionCurrency, sourceCurrency string,
) interfaces.FeeQuote {
	src := normalizeCurrency(sourceCurrency)
	// No conversion happened on the source side → no FX fee.
	if normalizeCurrency(transactionCurrency) == src {
		return interfaces.FeeQuote{Amount: decimal.Zero, Currency: src}
	}
	fee, ok := s.fees[src]
	if !ok {
		return interfaces.FeeQuote{Amount: decimal.Zero, Currency: src}
	}
	return interfaces.FeeQuote{
		Amount:   roundByCurrency(fee, src),
		Currency: src,
	}
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
