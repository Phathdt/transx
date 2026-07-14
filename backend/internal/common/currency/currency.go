// Package currency holds the ISO-4217 allow-list shared by every module that
// accepts a currency code (wallet accounts, transfer amounts). Keeping it here
// avoids each module re-declaring its own copy of the supported set.
package currency

import "strings"

// supported is the ISO-4217 allow-list the system accepts. Kept small and
// explicit; extend as new corridors are supported.
var supported = map[string]struct{}{
	"USD": {},
	"EUR": {},
	"VND": {},
}

// Normalize upper-cases and trims a currency code for comparison/storage.
func Normalize(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// IsSupported reports whether code is on the ISO-4217 allow-list.
func IsSupported(code string) bool {
	_, ok := supported[Normalize(code)]
	return ok
}
