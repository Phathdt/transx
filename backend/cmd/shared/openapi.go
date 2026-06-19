package shared

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/oaswrap/spec/adapter/fiberopenapi"
	"github.com/oaswrap/spec/option"
)

// NewOpenAPIRouter returns a route registrar that can also export the schema.
func NewOpenAPIRouter(app *fiber.App) fiberopenapi.Generator {
	return fiberopenapi.NewRouter(app,
		option.WithOpenAPIVersion("3.1.0"),
		option.WithTitle("transx API"),
		option.WithVersion("1.0.0"),
		option.WithDescription("transx API"),
		option.WithServer("/api/v1"),
		option.WithPathParser(apiV1PathParser{}),
		// Mark request fields whose validate tag contains "required" as required
		// in the schema, so the spec mirrors the runtime validation.
		option.WithReflectorConfig(option.RequiredPropByValidateTag()),
	)
}

type apiV1PathParser struct{}

func (apiV1PathParser) Parse(path string) (string, error) {
	path = strings.TrimPrefix(path, "/api/v1")
	if path == "" {
		path = "/"
	}

	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") && len(part) > 1 {
			parts[i] = "{" + strings.TrimPrefix(part, ":") + "}"
		}
	}

	return strings.Join(parts, "/"), nil
}
