package provider

import (
	"context"
	"fmt"
	"hash/fnv"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"transx/internal/modules/transfer/domain/entities"
)

// Fake client modes, configurable so the full external flow can be exercised
// end-to-end without a real provider API.
const (
	ModeAlwaysSuccess = "always_success"
	ModeAlwaysFailure = "always_failure"
	ModeAlwaysTimeout = "always_timeout"
	// ModeRandom picks SUCCESS or FAILURE deterministically from transfer_id
	// (~50/50). Submit and Query on the same id always agree, so Temporal UNKNOWN
	// polls stay consistent with the original Submit outcome.
	ModeRandom = "random"
)

// providerRejected is the failure reason the fake reports on failure outcomes.
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
// retry path. random maps transfer_id → SUCCESS|FAILURE stably.
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
	case ModeRandom:
		if randomFailure(transferID) {
			return entities.ProviderResult{
				Outcome: entities.ProviderFailure,
				Reason:  providerRejected,
			}, nil
		}
		return entities.ProviderResult{
			Outcome:     entities.ProviderSuccess,
			ReferenceID: "stub-" + transferID.String(),
		}, nil
	default: // ModeAlwaysSuccess
		return entities.ProviderResult{
			Outcome:     entities.ProviderSuccess,
			ReferenceID: "stub-" + transferID.String(),
		}, nil
	}
}

// randomFailure reports whether transferID should fail under ModeRandom. Uses
// a stable hash so repeated Submit/Query for the same id return the same
// outcome (required for the Bank gRPC stateless Query contract).
func randomFailure(transferID uuid.UUID) bool {
	h := fnv.New32a()
	_, _ = h.Write(transferID[:])
	return h.Sum32()%2 == 1
}
