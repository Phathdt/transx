package dto

import "github.com/shopspring/decimal"

// TransferNotificationContext is the data reloaded from the database to build a
// transfer notification. It is assembled by joining the transfer to its sender
// account and that account's user (the transfer.* event carries only the id).
type TransferNotificationContext struct {
	Reference     string
	Status        string
	FailureReason string
	Amount        decimal.Decimal
	Currency      string
	ToAccountRef  string
	// RecipientEmail is the EMAIL channel recipient (the sender's address).
	RecipientEmail string
	RecipientName  string
	// RecipientUserID is the PUSH channel recipient: a placeholder until a
	// device-token table exists, at which point real tokens plug in here.
	RecipientUserID string
}
