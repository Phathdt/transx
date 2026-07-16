package grpc

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"transx/internal/modules/transfer/domain/interfaces"
	bankv1 "transx/internal/platform/grpc/gen/bank/v1"
)

// BankServer is a stateless external payment provider (Submit/Query). It holds
// no operation/callback state: both RPCs derive outcomes via the injected
// ProviderClient so Query on any transfer_id returns the same result Submit
// would return right now (mode-driven fakes are deterministic per id).
type BankServer struct {
	bankv1.UnimplementedBankServiceServer
	provider interfaces.ProviderClient
}

// NewBankServer adapts a ProviderClient to the Bank gRPC surface.
func NewBankServer(provider interfaces.ProviderClient) *BankServer {
	return &BankServer{provider: provider}
}

func (s *BankServer) Submit(ctx context.Context, req *bankv1.SubmitRequest) (*bankv1.SubmitResponse, error) {
	transferID, amount, err := parseBankRequest(req.GetTransferId(), req.GetAmount())
	if err != nil {
		return nil, err
	}

	result, err := s.provider.Submit(ctx, transferID, amount, req.GetCurrency())
	if err != nil {
		// always_timeout: transient, so the caller retries rather than settles.
		return nil, status.Error(codes.DeadlineExceeded, err.Error())
	}
	return &bankv1.SubmitResponse{
		Outcome:     string(result.Outcome),
		ReferenceId: result.ReferenceID,
		Reason:      result.Reason,
	}, nil
}

// Query re-derives the outcome for transfer_id from the provider. The server
// holds no per-transfer state, so this is not a lookup of a recorded result —
// it recomputes the same deterministic outcome Submit would return.
func (s *BankServer) Query(ctx context.Context, req *bankv1.QueryRequest) (*bankv1.QueryResponse, error) {
	transferID, err := uuid.Parse(req.GetTransferId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid transfer_id %q: %v", req.GetTransferId(), err)
	}

	// Amount/currency do not affect mode-driven fake outcomes. random is
	// deterministic per transfer_id so Query matches Submit. Query passes
	// zero-value placeholders through the same client Submit uses.
	result, err := s.provider.Submit(ctx, transferID, decimal.Zero, "")
	if err != nil {
		return nil, status.Error(codes.DeadlineExceeded, err.Error())
	}
	return &bankv1.QueryResponse{
		Outcome:     string(result.Outcome),
		ReferenceId: result.ReferenceID,
		Reason:      result.Reason,
	}, nil
}

func parseBankRequest(transferIDRaw, amountRaw string) (uuid.UUID, decimal.Decimal, error) {
	transferID, err := uuid.Parse(transferIDRaw)
	if err != nil {
		return uuid.Nil, decimal.Decimal{}, status.Errorf(
			codes.InvalidArgument,
			"invalid transfer_id %q: %v",
			transferIDRaw,
			err,
		)
	}
	amount, err := decimal.NewFromString(amountRaw)
	if err != nil {
		return uuid.Nil, decimal.Decimal{}, status.Errorf(
			codes.InvalidArgument,
			"invalid amount %q: %v",
			amountRaw,
			err,
		)
	}
	return transferID, amount, nil
}
