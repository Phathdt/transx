package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/oaswrap/spec/adapter/fiberopenapi"
)

func RegisterRoutes(app *fiber.App) {
	_ = fiberopenapi.NewRouter(app)
}

func RegisterAllRoutesForSpec(r fiberopenapi.Router) {
}
