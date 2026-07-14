package interfaces

import (
	"context"

	"transx/internal/modules/wallet/domain/entities"
)

// ProviderAccountLookupClient validates external beneficiary accounts without
// coupling lookup reads to the transfer submission worker contract (that
// contract, interfaces.ProviderClient, lives in the transfer module — it
// submits money out through the provider, which is a transfer-owned concern).
type ProviderAccountLookupClient interface {
	LookupAccount(ctx context.Context, accountRef string) (*entities.AccountLookup, error)
}
