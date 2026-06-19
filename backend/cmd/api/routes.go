package api

import (
	"transx/cmd/api/handlers"
	authdto "transx/internal/modules/auth/application/dto"

	"github.com/gofiber/fiber/v2"
	"github.com/oaswrap/spec/adapter/fiberopenapi"
	"github.com/oaswrap/spec/option"
)

// RegisterRoutes wires the auth handlers onto the Fiber app for the running
// service. The spec router groups under /api/v1 to match the gateway prefix.
func RegisterRoutes(app *fiber.App, authH *handlers.AuthHandler) {
	RegisterAuthRoutes(fiberopenapi.NewRouter(app), authH)
}

// RegisterAllRoutesForSpec registers every route with nil handlers so the
// OpenAPI exporter can emit the full spec without wiring real dependencies.
func RegisterAllRoutesForSpec(r fiberopenapi.Router) {
	RegisterAuthRoutes(r, nil)
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
