package dto

// CreateAccountCommand is the POST /accounts request body.
type CreateAccountCommand struct {
	Name     string `json:"name"     validate:"max=255"`
	Currency string `json:"currency" validate:"required,iso4217"`
}

// AccountResponse is the wallet account view returned to clients. AccountRef is
// the external business id (ACC- + ULID); the internal UUID is never exposed.
type AccountResponse struct {
	AccountRef       string `json:"accountRef"`
	AvailableBalance string `json:"availableBalance"`
	HoldBalance      string `json:"holdBalance"`
	Currency         string `json:"currency"`
	Status           string `json:"status"`
}

// AccountLookupResponse is the compact lookup view used to validate a transfer
// beneficiary without exposing balances, internal UUIDs, user IDs, or emails.
type AccountLookupResponse struct {
	AccountRef string `json:"accountRef"`
	Currency   string `json:"currency"`
	Status     string `json:"status"`
	HolderName string `json:"holderName"`
}

// CreateTransferCommand is the POST /transfers request body. The idempotency key
// travels in the Idempotency-Key header (read separately by the handler), not in
// the body; it is declared here only so it appears in the OpenAPI spec.
type CreateTransferCommand struct {
	// IdempotencyKey documents the required Idempotency-Key header (a client-
	// generated UUID, uuidv7 recommended). It is header-only: BodyParser ignores
	// it (no json tag) and the handler reads it via c.Get.
	IdempotencyKey string `header:"Idempotency-Key" json:"-" required:"true" validate:"required,uuid"`

	FromAccountRef string `json:"fromAccountRef" validate:"required"`
	// ToAccountRef is required for INTERNAL transfers (an ACC- ref of an
	// in-system account) and is a free-text beneficiary id for EXTERNAL
	// transfers (validated per type in the service, not by a static tag).
	// nefield guards a self-transfer only when a destination is supplied.
	ToAccountRef string `json:"toAccountRef"   validate:"omitempty,nefield=FromAccountRef"`
	Amount       string `json:"amount"         validate:"required"`
	Currency     string `json:"currency"       validate:"required,iso4217"`
	TransferType string `json:"transferType"   validate:"omitempty,oneof=INTERNAL EXTERNAL"`
}

// TransferResponse is the transfer view returned to clients.
type TransferResponse struct {
	// TransferID is the business reference (ETN- for EXTERNAL, ITN- for INTERNAL,
	// followed by a ULID), not the internal UUID primary key.
	TransferID          string `json:"transferId"`
	Status              string `json:"status"`
	TransactionAmount   string `json:"transactionAmount"`
	TransactionCurrency string `json:"transactionCurrency"`
	SourceAmount        string `json:"sourceAmount,omitempty"`
	SourceCurrency      string `json:"sourceCurrency,omitempty"`
	DestinationAmount   string `json:"destinationAmount,omitempty"`
	DestinationCurrency string `json:"destinationCurrency,omitempty"`
	SourceFXRate        string `json:"sourceFxRate,omitempty"`
	DestinationFXRate   string `json:"destinationFxRate,omitempty"`
	FeeAmount           string `json:"feeAmount"`
	FeeCurrency         string `json:"feeCurrency"`
	FailureReason       string `json:"failureReason,omitempty"`
}

// TransferEventPayload is the canonical message body for transfer.* events
// (the outbox producer and the Kafka consumer share this contract). Only the
// transfer id travels on the wire; consumers reload state from the database.
type TransferEventPayload struct {
	TransferID string `json:"transferId"`
}

// AccountListResponse is the paginated list of accounts returned to clients.
type AccountListResponse struct {
	Data     []AccountResponse `json:"data"`
	Page     int               `json:"page"`
	PageSize int               `json:"pageSize"`
	Total    int64             `json:"total"`
}

// TransferListResponse is the paginated list of transfers returned to clients.
type TransferListResponse struct {
	Data     []TransferResponse `json:"data"`
	Page     int                `json:"page"`
	PageSize int                `json:"pageSize"`
	Total    int64              `json:"total"`
}

// ListAccountsQuery documents the GET /accounts query params for the OpenAPI
// spec. Validation/clamp happens in the service, not via these tags. Fields
// carry only the query tag (no json) so they render as query params, not a body.
type ListAccountsQuery struct {
	Page     int    `query:"page"`
	PageSize int    `query:"pageSize"`
	Currency string `query:"currency"`
	Status   string `query:"status"`
}

// ListTransfersQuery documents the GET /transfers query params for the OpenAPI
// spec. Validation/clamp happens in the service, not via these tags. Fields
// carry only the query tag (no json) so they render as query params, not a body.
type ListTransfersQuery struct {
	Page       int    `query:"page"`
	PageSize   int    `query:"pageSize"`
	Status     string `query:"status"`
	AccountRef string `query:"accountRef"`
}
