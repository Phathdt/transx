package fx

import (
	"context"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"transx/internal/modules/transfer/domain/interfaces"
	fxv1 "transx/internal/platform/grpc/gen/fx/v1"
)

// GRPCClient adapts the FX gRPC service to the wallet domain's FXService port.
// Decimal values cross the wire as strings to preserve precision; a FX-specific
// FailedPrecondition maps to ErrFXRateUnavailable so the consumer treats an
// unconfigured corridor as a business failure rather than a transient error.
type GRPCClient struct {
	client fxv1.FXServiceClient
}

var _ interfaces.FXService = (*GRPCClient)(nil)

func NewGRPCClient(client fxv1.FXServiceClient) *GRPCClient {
	return &GRPCClient{client: client}
}

func (c *GRPCClient) Quote(
	ctx context.Context,
	amount decimal.Decimal,
	fromCurrency, toCurrency string,
) (interfaces.FXQuote, error) {
	resp, err := c.client.Quote(ctx, &fxv1.QuoteRequest{
		Amount:       amount.String(),
		FromCurrency: fromCurrency,
		ToCurrency:   toCurrency,
	})
	if err != nil {
		if status.Code(err) == codes.FailedPrecondition {
			return interfaces.FXQuote{}, interfaces.ErrFXRateUnavailable
		}
		return interfaces.FXQuote{}, err
	}
	quoteAmount, err := decimal.NewFromString(resp.GetAmount())
	if err != nil {
		return interfaces.FXQuote{}, err
	}
	rate, err := decimal.NewFromString(resp.GetRate())
	if err != nil {
		return interfaces.FXQuote{}, err
	}
	return interfaces.FXQuote{
		Amount:   quoteAmount,
		Currency: resp.GetCurrency(),
		Rate:     rate,
		Source:   resp.GetSource(),
	}, nil
}

func (c *GRPCClient) QuoteFee(
	ctx context.Context,
	transactionCurrency, sourceCurrency string,
) (interfaces.FeeQuote, error) {
	resp, err := c.client.QuoteFee(ctx, &fxv1.QuoteFeeRequest{
		TransactionCurrency: transactionCurrency,
		SourceCurrency:      sourceCurrency,
	})
	if err != nil {
		if status.Code(err) == codes.FailedPrecondition {
			return interfaces.FeeQuote{}, interfaces.ErrFXRateUnavailable
		}
		return interfaces.FeeQuote{}, err
	}
	feeAmount, err := decimal.NewFromString(resp.GetAmount())
	if err != nil {
		return interfaces.FeeQuote{}, err
	}
	return interfaces.FeeQuote{
		Amount:   feeAmount,
		Currency: resp.GetCurrency(),
	}, nil
}
