package provider

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"transx/internal/modules/transfer/domain/entities"
)

// StubHandler serves POST /submit for the stub-provider service. It reuses
// FakeProviderClient for the mode-driven outcome (success/failure/timeout) so
// the fake behaviour lives in one place, then maps the result onto the HTTP
// contract. always_timeout surfaces as 504 so the client retries.
type StubHandler struct {
	fake *FakeProviderClient
}

var stubExternalAccounts = map[string]AccountLookupResponse{
	"EXT-ACME-USD-001": {
		AccountRef: "EXT-ACME-USD-001",
		Currency:   "USD",
		Status:     "ACTIVE",
		HolderName: "Acme Treasury",
	},
	"EXT-GLOBEX-EUR-001": {
		AccountRef: "EXT-GLOBEX-EUR-001",
		Currency:   "EUR",
		Status:     "ACTIVE",
		HolderName: "Globex Settlement",
	},
}

// NewStubHandler builds a handler driven by mode (always_success |
// always_failure | always_timeout); an empty mode defaults to success.
func NewStubHandler(mode string) *StubHandler {
	return &StubHandler{fake: NewFakeProviderClient(mode)}
}

// LookupAccount handles GET /accounts/:accountRef for provider beneficiary validation.
func (h *StubHandler) LookupAccount(c *fiber.Ctx) error {
	account, ok := stubExternalAccounts[c.Params("accountRef")]
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "account not found"})
	}
	return c.JSON(account)
}

// Submit handles POST /submit: decode the request, run the fake, and return a
// definitive outcome as 200 or a transient failure as 504.
func (h *StubHandler) Submit(c *fiber.Ctx) error {
	var req SubmitRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	transferID, err := uuid.Parse(req.TransferID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid transfer_id"})
	}

	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid amount"})
	}

	result, err := h.fake.Submit(c.Context(), transferID, amount, req.Currency)
	if err != nil {
		// Transient (always_timeout): 504 so the client treats it as retryable.
		return c.Status(fiber.StatusGatewayTimeout).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(toSubmitResponse(result))
}

// toSubmitResponse maps a settled ProviderResult onto the HTTP wire type.
func toSubmitResponse(r entities.ProviderResult) SubmitResponse {
	return SubmitResponse{
		Outcome:     string(r.Outcome),
		ReferenceID: r.ReferenceID,
		Reason:      r.Reason,
	}
}

// SubmitPath exposes the submission route path for the runner registering it.
func SubmitPath() string { return submitPath }

// AccountLookupPath exposes the lookup route path for the stub-provider runner.
func AccountLookupPath() string { return accountLookupPathPrefix + ":accountRef" }
