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

// AccountListResponse is the paginated list of accounts returned to clients.
type AccountListResponse struct {
	Data     []AccountResponse `json:"data"`
	Page     int               `json:"page"`
	PageSize int               `json:"pageSize"`
	Total    int64             `json:"total"`
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
