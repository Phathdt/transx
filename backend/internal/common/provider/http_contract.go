package provider

// HTTP provider contract shared by the consumer-side HTTP client and the
// stub-provider HTTP server. Keeping the wire types in one place means the
// client and server can never drift on field names.

// SubmitRequest is the POST /submit request body: the transfer to submit to the
// payment provider. Amount is a decimal string to avoid float rounding.
type SubmitRequest struct {
	TransferID string `json:"transfer_id"`
	Amount     string `json:"amount"`
	Currency   string `json:"currency"`
}

// SubmitResponse is the POST /submit success body carrying a definitive outcome.
// A transient condition is never expressed here — it is signalled by a non-2xx
// status so the client returns an error and the worker retries.
type SubmitResponse struct {
	Outcome     string `json:"outcome"`                // SUCCESS | FAILURE
	ReferenceID string `json:"reference_id,omitempty"` // set on SUCCESS
	Reason      string `json:"reason,omitempty"`       // set on FAILURE
}

// AccountLookupResponse is the GET /accounts/{accountRef} success body. It is
// intentionally compact so beneficiary validation does not expose balances.
type AccountLookupResponse struct {
	AccountRef string `json:"account_ref"`
	Currency   string `json:"currency"`
	Status     string `json:"status"`
	HolderName string `json:"holder_name"`
}

const (
	// submitPath is the provider submission endpoint, shared by client and server.
	submitPath = "/submit"
	// accountLookupPathPrefix is the provider account lookup endpoint prefix.
	accountLookupPathPrefix = "/accounts/"
)
