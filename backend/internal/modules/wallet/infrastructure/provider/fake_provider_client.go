package provider

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"transx/internal/modules/wallet/domain/entities"
)

// Fake client modes, configurable so the full external flow can be exercised
// end-to-end without a real provider API.
const (
	ModeAlwaysSuccess = "always_success"
	ModeAlwaysFailure = "always_failure"
	ModeAlwaysTimeout = "always_timeout"
)

// providerRejected is the failure reason the fake reports on always_failure.
const providerRejected = "PROVIDER_REJECTED"

// FakeProviderClient is a mode-driven stub implementing interfaces.ProviderClient.
type FakeProviderClient struct {
	mode string
}

// NewFakeProviderClient builds a fake client; an empty mode defaults to success.
func NewFakeProviderClient(mode string) *FakeProviderClient {
	if mode == "" {
		mode = ModeAlwaysSuccess
	}
	return &FakeProviderClient{mode: mode}
}

// Submit returns a deterministic outcome based on the configured mode. The
// reference id is derived from the transfer id (no randomness) so results are
// reproducible. always_timeout returns an error to drive the worker's transient
// retry path.
func (c *FakeProviderClient) Submit(
	_ context.Context,
	transferID uuid.UUID,
	_ decimal.Decimal,
	_ string,
) (entities.ProviderResult, error) {
	switch c.mode {
	case ModeAlwaysFailure:
		return entities.ProviderResult{
			Outcome: entities.ProviderFailure,
			Reason:  providerRejected,
		}, nil
	case ModeAlwaysTimeout:
		return entities.ProviderResult{}, fmt.Errorf("provider submit timed out for transfer %s", transferID)
	default: // ModeAlwaysSuccess
		return entities.ProviderResult{
			Outcome:     entities.ProviderSuccess,
			ReferenceID: "stub-" + transferID.String(),
		}, nil
	}
}
