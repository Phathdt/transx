package dto

import "github.com/shopspring/decimal"

// TransferNotificationContext is the data reloaded from the database to build a
// transfer notification and inbox items. It is assembled by joining the
// transfer to its sender account/user and, when present, the destination
// account/user (the transfer.* event carries only the id).
type TransferNotificationContext struct {
	Reference     string
	Status        string
	FailureReason string
	Amount        decimal.Decimal
	Currency      string
	ToAccountRef  string
	// TransferType is INTERNAL or EXTERNAL; used for inbox recipient rules.
	TransferType string
	// RecipientEmail is the EMAIL channel recipient (the sender's address).
	RecipientEmail string
	RecipientName  string
	// RecipientUserID is the sender's user id. Used as the PUSH channel
	// recipient placeholder and as the always-present inbox recipient.
	RecipientUserID string
	// ToUserID is the destination account's owner when the destination is an
	// in-system account (typical INTERNAL). Empty for EXTERNAL free-text refs.
	ToUserID string
}
