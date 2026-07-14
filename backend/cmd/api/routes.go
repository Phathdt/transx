package api

import (
	"transx/cmd/api/handlers"
	authdto "transx/internal/modules/auth/application/dto"
	transferdto "transx/internal/modules/transfer/application/dto"
	walletdto "transx/internal/modules/wallet/application/dto"

	"github.com/gofiber/fiber/v2"
	"github.com/oaswrap/spec/adapter/fiberopenapi"
	"github.com/oaswrap/spec/option"
)

// RegisterRoutes wires the auth handlers onto the Fiber app for the running auth
// service. The spec router groups under /api/v1 to match the gateway prefix.
func RegisterRoutes(app *fiber.App, authH *handlers.AuthHandler) {
	RegisterAuthRoutes(fiberopenapi.NewRouter(app), authH)
}

// RegisterWalletRoutes wires the wallet handlers onto the Fiber app for the
// running wallet service. Kept separate from RegisterRoutes so each binary only
// registers the routes it actually serves (a shared registrar would force a nil
// handler for the other service and panic at request time).
func RegisterWalletRoutes(app *fiber.App, walletH *handlers.WalletHandler) {
	registerWalletRoutes(fiberopenapi.NewRouter(app), walletH)
}

// RegisterTransferRoutes wires the transfer handlers onto the Fiber app for the
// running transfer service. Kept separate from RegisterWalletRoutes so each
// binary only registers the routes it actually serves (a shared registrar
// would force a nil handler for the other service and panic at request time).
func RegisterTransferRoutes(app *fiber.App, transferH *handlers.TransferHandler) {
	registerTransferRoutes(fiberopenapi.NewRouter(app), transferH)
}

// RegisterAllRoutesForSpec registers every route with nil handlers so the
// OpenAPI exporter can emit the full spec without wiring real dependencies. Nil
// handlers are safe here because the exporter never invokes them.
func RegisterAllRoutesForSpec(r fiberopenapi.Router) {
	RegisterAuthRoutes(r, nil)
	registerWalletRoutes(r, nil)
	registerTransferRoutes(r, nil)
}

// RegisterAuthRoutes wires the auth-service routes: login + the ForwardAuth
// check endpoint. Passing a nil handler registers the route for spec export
// only (no live handler attached).
func RegisterAuthRoutes(r fiberopenapi.Router, authH *handlers.AuthHandler) {
	v1 := r.Group("/api/v1")

	var login, check fiber.Handler
	if authH != nil {
		login = authH.Login
		check = authH.Check
	}

	v1.Post("/login", login).With(
		option.Tags("auth"),
		option.OperationID("login"),
		option.Summary("Authenticate with email and password, returns a JWT"),
		option.Request(new(authdto.LoginCommand), option.ContentRequired()),
		option.Response(fiber.StatusOK, new(authdto.LoginResponse)),
		option.Response(fiber.StatusBadRequest, new(handlers.ErrorResponse)),
		option.Response(fiber.StatusUnauthorized, new(handlers.ErrorResponse)),
		option.Response(fiber.StatusInternalServerError, new(handlers.ErrorResponse)),
	)

	// ForwardAuth backend used by Traefik. On success it returns 200 with the
	// X-User-Id response header; on an invalid/missing token it returns 401.
	v1.Get("/check", check).With(
		option.Tags("auth"),
		option.OperationID("check"),
		option.Summary("ForwardAuth check: verify bearer token, echo X-User-Id"),
		option.Response(fiber.StatusOK, nil),
		option.Response(fiber.StatusUnauthorized, new(handlers.ErrorResponse)),
	)
}

// registerWalletRoutes wires the wallet-service routes onto the spec router.
// A nil handler registers the route for spec export only.
func registerWalletRoutes(r fiberopenapi.Router, walletH *handlers.WalletHandler) {
	v1 := r.Group("/api/v1")

	var createAccount, getAccount, lookupAccount, listAccounts fiber.Handler
	if walletH != nil {
		createAccount = walletH.CreateAccount
		getAccount = walletH.GetAccount
		lookupAccount = walletH.LookupAccount
		listAccounts = walletH.ListAccounts
	}

	v1.Post("/accounts", createAccount).With(
		option.Tags("wallet"),
		option.OperationID("createAccount"),
		option.Summary("Create a wallet account for the caller"),
		option.Request(new(walletdto.CreateAccountCommand), option.ContentRequired()),
		option.Response(fiber.StatusCreated, new(walletdto.AccountResponse)),
		option.Response(fiber.StatusBadRequest, new(handlers.ErrorResponse)),
		option.Response(fiber.StatusUnauthorized, new(handlers.ErrorResponse)),
		option.Response(fiber.StatusInternalServerError, new(handlers.ErrorResponse)),
	)

	v1.Get("/accounts", listAccounts).With(
		option.Tags("wallet"),
		option.OperationID("listAccounts"),
		option.Summary("List the caller's wallet accounts, paginated, with optional currency and status filters"),
		option.Request(new(walletdto.ListAccountsQuery)),
		option.Response(fiber.StatusOK, new(walletdto.AccountListResponse)),
		option.Response(fiber.StatusBadRequest, new(handlers.ErrorResponse)),
		option.Response(fiber.StatusUnauthorized, new(handlers.ErrorResponse)),
		option.Response(fiber.StatusInternalServerError, new(handlers.ErrorResponse)),
	)

	v1.Get("/accounts/:accountType/:accountRef", lookupAccount).With(
		option.Tags("wallet"),
		option.OperationID("lookupAccount"),
		option.Summary("Look up an internal or external account for transfer beneficiary validation"),
		option.Response(fiber.StatusOK, new(walletdto.AccountLookupResponse)),
		option.Response(fiber.StatusBadRequest, new(handlers.ErrorResponse)),
		option.Response(fiber.StatusUnauthorized, new(handlers.ErrorResponse)),
		option.Response(fiber.StatusNotFound, new(handlers.ErrorResponse)),
		option.Response(fiber.StatusBadGateway, new(handlers.ErrorResponse)),
	)

	v1.Get("/accounts/:accountRef", getAccount).With(
		option.Tags("wallet"),
		option.OperationID("getAccount"),
		option.Summary("Get a wallet account balance by its accountRef (ACC- + ULID)"),
		option.Response(fiber.StatusOK, new(walletdto.AccountResponse)),
		option.Response(fiber.StatusUnauthorized, new(handlers.ErrorResponse)),
		option.Response(fiber.StatusNotFound, new(handlers.ErrorResponse)),
		option.Response(fiber.StatusInternalServerError, new(handlers.ErrorResponse)),
	)
}

// registerTransferRoutes wires the transfer-service routes onto the spec
// router. A nil handler registers the route for spec export only.
func registerTransferRoutes(r fiberopenapi.Router, transferH *handlers.TransferHandler) {
	v1 := r.Group("/api/v1")

	var createTransfer, getTransfer, listTransfers fiber.Handler
	if transferH != nil {
		createTransfer = transferH.CreateTransfer
		getTransfer = transferH.GetTransfer
		listTransfers = transferH.ListTransfers
	}

	v1.Post("/transfers", createTransfer).With(
		option.Tags("wallet"),
		option.OperationID("createTransfer"),
		option.Summary("Create a transfer (idempotent via Idempotency-Key). INTERNAL needs toAccountRef (an ACC- account ref); EXTERNAL omits it or carries a free-text beneficiary id (provider is set from server config)."),
		option.Request(new(transferdto.CreateTransferCommand), option.ContentRequired()),
		option.Response(fiber.StatusAccepted, new(transferdto.TransferResponse)),
		option.Response(fiber.StatusBadRequest, new(handlers.ErrorResponse)),
		option.Response(fiber.StatusUnauthorized, new(handlers.ErrorResponse)),
		option.Response(fiber.StatusForbidden, new(handlers.ErrorResponse)),
		option.Response(fiber.StatusConflict, new(handlers.ErrorResponse)),
		option.Response(fiber.StatusUnprocessableEntity, new(handlers.ErrorResponse)),
		option.Response(fiber.StatusInternalServerError, new(handlers.ErrorResponse)),
	)

	v1.Get("/transfers", listTransfers).With(
		option.Tags("wallet"),
		option.OperationID("listTransfers"),
		option.Summary("List the caller's transfers, paginated, with optional status and accountRef filters"),
		option.Request(new(transferdto.ListTransfersQuery)),
		option.Response(fiber.StatusOK, new(transferdto.TransferListResponse)),
		option.Response(fiber.StatusBadRequest, new(handlers.ErrorResponse)),
		option.Response(fiber.StatusUnauthorized, new(handlers.ErrorResponse)),
		option.Response(fiber.StatusInternalServerError, new(handlers.ErrorResponse)),
	)

	v1.Get("/transfers/:transferId", getTransfer).With(
		option.Tags("wallet"),
		option.OperationID("getTransfer"),
		option.Summary("Get a transfer by its business reference (ETN- for EXTERNAL, ITN- for INTERNAL, followed by a ULID)"),
		option.Response(fiber.StatusOK, new(transferdto.TransferResponse)),
		option.Response(fiber.StatusBadRequest, new(handlers.ErrorResponse)),
		option.Response(fiber.StatusUnauthorized, new(handlers.ErrorResponse)),
		option.Response(fiber.StatusNotFound, new(handlers.ErrorResponse)),
		option.Response(fiber.StatusInternalServerError, new(handlers.ErrorResponse)),
	)
}
