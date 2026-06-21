package entities

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// AccountStatus enumerates the lifecycle states of a wallet account. Only an
// ACTIVE account can be debited or credited.
type AccountStatus string

const (
	AccountStatusActive AccountStatus = "ACTIVE"
	AccountStatusFrozen AccountStatus = "FROZEN"
	AccountStatusClosed AccountStatus = "CLOSED"
)

// Account is a user's wallet holding a balance in a single currency.
// AvailableBalance is spendable; HoldBalance is reserved for in-flight holds.
// ID is the internal UUID primary key; Ref is the external business id
// (ACC- + ULID) exposed to clients so the UUID never leaves the system.
type Account struct {
	ID               uuid.UUID
	Ref              string
	UserID           uuid.UUID
	Name             string
	Currency         string
	AvailableBalance decimal.Decimal
	HoldBalance      decimal.Decimal
	Status           AccountStatus
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// AccountLookup is the compact transfer-oriented account view. It intentionally
// omits balances, user ids, and internal UUIDs.
type AccountLookup struct {
	AccountRef string
	Currency   string
	Status     string
	HolderName string
}

// IsActive reports whether the account can take part in a transfer.
func (a *Account) IsActive() bool { return a.Status == AccountStatusActive }
