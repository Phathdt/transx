package interfaces

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"transx/internal/modules/wallet/domain/entities"
)

// ProviderClient submits an external transfer to a payment provider.
//
// Error vs result is the transient/permanent contract the provider worker relies
// on: a returned error means a transient condition (timeout/network) and the
// worker retries through the delayed-retry tiers; a (result, nil) return is a
// definitive business outcome (SUCCESS or FAILURE) the worker settles on.
//
// transferID is the idempotency key for the submission: the worker may re-invoke
// Submit on retry or Kafka redelivery, so an implementation backed by a real
// provider MUST dedupe on transferID to avoid sending funds twice.
type ProviderClient interface {
	Submit(
		ctx context.Context,
		transferID uuid.UUID,
		amount decimal.Decimal,
		currency string,
	) (entities.ProviderResult, error)
}
