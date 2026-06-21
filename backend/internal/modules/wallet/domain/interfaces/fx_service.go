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

// FeeQuote is the FX conversion fee charged on the source account, already
// rounded to the source currency scale. Currency is always the source currency.
type FeeQuote struct {
	Amount   decimal.Decimal
	Currency string
}

// FXService quotes transaction money into a target currency. Implementations may
// reach a remote service, so every method can fail; callers must not trust
// client-supplied settlement values.
type FXService interface {
	Quote(ctx context.Context, amount decimal.Decimal, fromCurrency, toCurrency string) (FXQuote, error)
	// QuoteFee returns the flat FX fee charged in the source currency when a
	// transfer converts out of it. A same-currency corridor (transactionCurrency
	// == sourceCurrency) means no conversion happened, so the fee is zero.
	QuoteFee(ctx context.Context, transactionCurrency, sourceCurrency string) (FeeQuote, error)
}
