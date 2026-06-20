package entities

// ProviderOutcome is the definitive business result of an external provider
// submission. A transient condition (timeout/network) is NOT an outcome — it is
// signalled as an error so the worker retries instead of settling.
type ProviderOutcome string

const (
	ProviderSuccess ProviderOutcome = "SUCCESS"
	ProviderFailure ProviderOutcome = "FAILURE"
)

// ProviderResult is what a provider returns for a settled submission.
type ProviderResult struct {
	Outcome     ProviderOutcome
	ReferenceID string // set on SUCCESS
	Reason      string // set on FAILURE (e.g. "PROVIDER_REJECTED")
}
