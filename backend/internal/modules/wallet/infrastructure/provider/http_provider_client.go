package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"transx/internal/modules/wallet/domain/entities"
)

// defaultHTTPTimeout bounds a single provider submission. On expiry the client
// returns an error, which the worker treats as transient and retries.
const defaultHTTPTimeout = 10 * time.Second

// HTTPProviderClient submits external transfers to a provider over HTTP. It
// implements interfaces.ProviderClient: a (result, nil) return is a definitive
// outcome the worker settles on; any error (non-2xx, timeout, network, decode)
// is transient and drives the worker's retry tiers.
type HTTPProviderClient struct {
	baseURL string
	client  *http.Client
}

// NewHTTPProviderClient builds a client targeting baseURL (e.g.
// "http://stub-provider:4100"). A zero timeout falls back to defaultHTTPTimeout.
func NewHTTPProviderClient(baseURL string, timeout time.Duration) *HTTPProviderClient {
	if timeout <= 0 {
		timeout = defaultHTTPTimeout
	}
	return &HTTPProviderClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: timeout},
	}
}

// Submit POSTs the transfer to baseURL+/submit and maps the response onto the
// ProviderClient contract. transferID is the idempotency key: a real provider
// must dedupe on it, so the worker can safely re-invoke Submit on retry.
func (c *HTTPProviderClient) Submit(
	ctx context.Context,
	transferID uuid.UUID,
	amount decimal.Decimal,
	currency string,
) (entities.ProviderResult, error) {
	body, err := json.Marshal(SubmitRequest{
		TransferID: transferID.String(),
		Amount:     amount.String(),
		Currency:   currency,
	})
	if err != nil {
		return entities.ProviderResult{}, fmt.Errorf("provider: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+submitPath, bytes.NewReader(body))
	if err != nil {
		return entities.ProviderResult{}, fmt.Errorf("provider: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		// Timeout/network error: transient, retry.
		return entities.ProviderResult{}, fmt.Errorf("provider: submit transfer %s: %w", transferID, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Non-2xx (incl. the stub's always_timeout 504): transient, retry.
		return entities.ProviderResult{}, fmt.Errorf(
			"provider: submit transfer %s: unexpected status %d", transferID, resp.StatusCode,
		)
	}

	var out SubmitResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return entities.ProviderResult{}, fmt.Errorf("provider: decode response: %w", err)
	}

	return entities.ProviderResult{
		Outcome:     entities.ProviderOutcome(out.Outcome),
		ReferenceID: out.ReferenceID,
		Reason:      out.Reason,
	}, nil
}
