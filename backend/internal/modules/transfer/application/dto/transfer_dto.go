package dto

// CreateTransferCommand is the POST /transfers request body. The idempotency key
// travels in the Idempotency-Key header (read separately by the handler), not in
// the body; it is declared here only so it appears in the OpenAPI spec.
type CreateTransferCommand struct {
	// IdempotencyKey documents the required Idempotency-Key header (a client-
	// generated UUID, uuidv7 recommended). It is header-only: BodyParser ignores
	// it (no json tag) and the handler reads it via c.Get.
	IdempotencyKey string `header:"Idempotency-Key" json:"-" required:"true" validate:"required,uuid"`

	FromAccountRef string `json:"fromAccountRef"      validate:"required"`
	// ToAccountRef is required for INTERNAL transfers (an ACC- ref of an
	// in-system account) and is a free-text beneficiary id for EXTERNAL
	// transfers (validated per type in the service, not by a static tag).
	// nefield guards a self-transfer only when a destination is supplied.
	ToAccountRef string `json:"toAccountRef"        validate:"omitempty,nefield=FromAccountRef"`
	Amount       string `json:"amount"              validate:"required"`
	Currency     string `json:"currency"            validate:"required,iso4217"`
	TransferType string `json:"transferType"        validate:"omitempty,oneof=INTERNAL EXTERNAL"`
	// Message is a user-supplied transfer note. The frontend pre-fills a template
	// and the user may edit it, but it must not be empty. It is descriptive only:
	// it does not feed the idempotency request hash.
	Message string `json:"message"             validate:"required,max=255"`
	// ExecuteAt is optional (RFC3339). When set, the transfer starts SCHEDULED
	// and no money moves until this time; the caller can cancel it beforehand
	// via POST /transfers/{transferId}/cancel. Omitted means immediate (today's
	// behavior). Horizon and future-only checks happen in the service, not here,
	// so the error message can name the actual bound (now, now+90d).
	ExecuteAt string `json:"executeAt,omitempty" validate:"omitempty"`
}

// TransferResponse is the transfer view returned to clients.
type TransferResponse struct {
	// TransferID is the business reference (ETN- for EXTERNAL, ITN- for INTERNAL,
	// followed by a ULID), not the internal UUID primary key.
	TransferID          string `json:"transferId"`
	Status              string `json:"status"`
	FromAccountRef      string `json:"fromAccountRef,omitempty"`
	ToAccountRef        string `json:"toAccountRef,omitempty"`
	ToAccountName       string `json:"toAccountName,omitempty"`
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
	Message             string `json:"message,omitempty"`
	FailureReason       string `json:"failureReason,omitempty"`
	// ExecuteAt is set (RFC3339) only for a transfer created with a future
	// execute time; empty for an immediate transfer.
	ExecuteAt string `json:"executeAt,omitempty"`
}

// TransferEventPayload is the canonical message body for transfer.* events
// (the outbox producer and the Kafka consumer share this contract). Only the
// transfer id travels on the wire; consumers reload state from the database.
type TransferEventPayload struct {
	TransferID string `json:"transferId"`
}

// TransferListResponse is the paginated list of transfers returned to clients.
type TransferListResponse struct {
	Data     []TransferResponse `json:"data"`
	Page     int                `json:"page"`
	PageSize int                `json:"pageSize"`
	Total    int64              `json:"total"`
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
