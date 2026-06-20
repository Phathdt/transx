package dto

// CreateAccountCommand is the POST /accounts request body.
type CreateAccountCommand struct {
	Name     string `json:"name"     validate:"max=255"`
	Currency string `json:"currency" validate:"required,iso4217"`
}

// AccountResponse is the wallet account view returned to clients.
type AccountResponse struct {
	AccountID        string `json:"accountId"`
	AvailableBalance string `json:"availableBalance"`
	HoldBalance      string `json:"holdBalance"`
	Currency         string `json:"currency"`
	Status           string `json:"status"`
}

// CreateTransferCommand is the POST /transfers request body. The idempotency key
// travels in the Idempotency-Key header (read separately by the handler), not in
// the body; it is declared here only so it appears in the OpenAPI spec.
type CreateTransferCommand struct {
	// IdempotencyKey documents the required Idempotency-Key header (a client-
	// generated UUID, uuidv7 recommended). It is header-only: BodyParser ignores
	// it (no json tag) and the handler reads it via c.Get.
	IdempotencyKey string `header:"Idempotency-Key" json:"-" required:"true" validate:"required,uuid"`

	FromAccountID string `json:"fromAccountId" validate:"required,uuid"`
	// ToAccountID is required for INTERNAL transfers and omitted for EXTERNAL
	// (validated per type in the service, not by a static tag). nefield guards a
	// self-transfer only when a destination is supplied.
	ToAccountID  string `json:"toAccountId"   validate:"omitempty,uuid,nefield=FromAccountID"`
	Amount       string `json:"amount"        validate:"required,number"`
	Currency     string `json:"currency"      validate:"required,iso4217"`
	TransferType string `json:"transferType"  validate:"omitempty,oneof=INTERNAL EXTERNAL"`
}

// TransferResponse is the transfer view returned to clients.
type TransferResponse struct {
	// TransferID is the business reference (ETN- for EXTERNAL, ITN- for INTERNAL,
	// followed by a ULID), not the internal UUID primary key.
	TransferID    string `json:"transferId"`
	Status        string `json:"status"`
	Amount        string `json:"amount"`
	Currency      string `json:"currency"`
	FailureReason string `json:"failureReason,omitempty"`
}

// TransferEventPayload is the canonical message body for transfer.* events
// (the outbox producer and the Kafka consumer share this contract). Only the
// transfer id travels on the wire; consumers reload state from the database.
type TransferEventPayload struct {
	TransferID string `json:"transferId"`
}
