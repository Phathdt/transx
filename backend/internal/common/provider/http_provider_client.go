package provider

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/imroc/req/v3"
	"github.com/shopspring/decimal"

	"transx/internal/common/apperror"
	transferentities "transx/internal/modules/transfer/domain/entities"
	walletentities "transx/internal/modules/wallet/domain/entities"
)

// defaultHTTPTimeout bounds a single provider submission. On expiry the client
// returns an error, which the worker treats as transient and retries.
const defaultHTTPTimeout = 10 * time.Second

// HTTPProviderClient submits external transfers to a provider over HTTP. It
// implements interfaces.ProviderClient: a (result, nil) return is a definitive
// outcome the worker settles on; any error (non-2xx, timeout, network, decode)
// is transient and drives the worker's retry tiers.
type HTTPProviderClient struct {
	client *req.Client
}

// NewHTTPProviderClient builds a client targeting baseURL (e.g.
// "http://stub-provider:4100"). A zero timeout falls back to defaultHTTPTimeout.
func NewHTTPProviderClient(baseURL string, timeout time.Duration) *HTTPProviderClient {
	if timeout <= 0 {
		timeout = defaultHTTPTimeout
	}
	client := req.C().
		SetBaseURL(baseURL).
		SetTimeout(timeout)
	return &HTTPProviderClient{client: client}
}

// LookupAccount reads beneficiary metadata from baseURL+/accounts/{accountRef}.
// A provider 404 is a definitive business miss; transport and server errors are
// upstream failures for the API layer.
func (c *HTTPProviderClient) LookupAccount(
	ctx context.Context,
	accountRef string,
) (*walletentities.AccountLookup, error) {
	var out AccountLookupResponse
	resp, err := c.client.R().
		SetContext(ctx).
		SetSuccessResult(&out).
		Get(accountLookupPathPrefix + accountRef)
	if err != nil {
		return nil, apperror.NewBadGatewayError("provider lookup failed", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if !resp.IsSuccessState() {
		return nil, apperror.NewBadGatewayError(
			"provider lookup failed",
			fmt.Errorf("provider: lookup account %s: unexpected status %d", accountRef, resp.StatusCode),
		)
	}

	return &walletentities.AccountLookup{
		AccountRef: out.AccountRef,
		Currency:   out.Currency,
		Status:     out.Status,
		HolderName: out.HolderName,
	}, nil
}

// Submit POSTs the transfer to baseURL+/submit and maps the response onto the
// ProviderClient contract. transferID is the idempotency key: a real provider
// must dedupe on it, so the worker can safely re-invoke Submit on retry.
func (c *HTTPProviderClient) Submit(
	ctx context.Context,
	transferID uuid.UUID,
	amount decimal.Decimal,
	currency string,
) (transferentities.ProviderResult, error) {
	var out SubmitResponse
	resp, err := c.client.R().
		SetContext(ctx).
		SetBody(SubmitRequest{
			TransferID: transferID.String(),
			Amount:     amount.String(),
			Currency:   currency,
		}).
		SetSuccessResult(&out).
		Post(submitPath)
	if err != nil {
		// Timeout/network/decode error: transient, retry.
		return transferentities.ProviderResult{}, fmt.Errorf("provider: submit transfer %s: %w", transferID, err)
	}
	if !resp.IsSuccessState() {
		// Non-2xx (incl. the stub's always_timeout 504): transient, retry.
		return transferentities.ProviderResult{}, fmt.Errorf(
			"provider: submit transfer %s: unexpected status %d", transferID, resp.StatusCode,
		)
	}

	return transferentities.ProviderResult{
		Outcome:     transferentities.ProviderOutcome(out.Outcome),
		ReferenceID: out.ReferenceID,
		Reason:      out.Reason,
	}, nil
}
