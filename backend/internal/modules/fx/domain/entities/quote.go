package entities

import "github.com/shopspring/decimal"

// FXQuote is a frozen quote for posting a transaction amount into one account's
// base currency.
type FXQuote struct {
	Amount   decimal.Decimal
	Currency string
	Rate     decimal.Decimal
	Source   string
}

// FeeQuote is the flat cross-currency fee charged in the source currency.
type FeeQuote struct {
	Amount   decimal.Decimal
	Currency string
}
