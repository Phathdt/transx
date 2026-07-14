// Package accountref defines the wallet account external business id (ACC- +
// ULID). It is shared because the transfer module validates a caller-supplied
// account ref (fromAccountRef/toAccountRef) without owning the accounts table
// itself; the wallet module is still the sole owner of account rows.
package accountref

import (
	"regexp"

	"github.com/oklog/ulid/v2"
)

// Pattern matches an account ref: an ACC- prefix followed by a 26-char
// Crockford base32 ULID (the alphabet excludes I, L, O, U).
var Pattern = regexp.MustCompile(`^ACC-[0-9A-HJKMNP-TV-Z]{26}$`)

// Valid reports whether ref matches the account reference format.
func Valid(ref string) bool {
	return Pattern.MatchString(ref)
}

// New generates an account's external business id: ACC- + ULID. The ULID is
// generated at the application layer (time + entropy), independent of the
// DB-assigned UUID primary key, mirroring the transfer reference scheme.
func New() string {
	return "ACC-" + ulid.Make().String()
}
