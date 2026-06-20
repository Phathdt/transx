package services

import "strings"

// supportedCurrencies is the ISO-4217 allow-list the wallet accepts. Kept small
// and explicit; extend as new corridors are supported.
var supportedCurrencies = map[string]struct{}{
	"USD": {},
	"EUR": {},
	"VND": {},
}

// normalizeCurrency upper-cases and trims a currency code for comparison/storage.
func normalizeCurrency(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// isSupportedCurrency reports whether code is on the ISO-4217 allow-list.
func isSupportedCurrency(code string) bool {
	_, ok := supportedCurrencies[normalizeCurrency(code)]
	return ok
}
