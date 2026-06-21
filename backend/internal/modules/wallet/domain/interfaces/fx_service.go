package interfaces

import (
	"context"
	"errors"

	"github.com/shopspring/decimal"
)

// ErrFXRateUnavailable means the requested currency corridor is not configured.
// The consumer treats it as a business failure, not a transient retry condition.
var ErrFXRateUnavailable = errors.New("fx rate unavailable")

// FXQuote is a frozen quote for posting a transaction amount into one account's
// base currency.
type FXQuote struct {
	Amount   decimal.Decimal
	Currency string
	Rate     decimal.Decimal
	Source   string
}

// FXService quotes transaction money into a target currency. Implementations must
// be synchronous and must not trust client-supplied settlement values.
type FXService interface {
	Quote(ctx context.Context, amount decimal.Decimal, fromCurrency, toCurrency string) (FXQuote, error)
}
