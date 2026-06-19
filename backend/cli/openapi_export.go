package cli

import (
	"fmt"

	cmdapi "transx/cmd/api"
	"transx/cmd/shared"

	"github.com/gofiber/fiber/v2"
	"github.com/urfave/cli/v2"
)

func RunOpenAPIExport(c *cli.Context) error {
	output := c.String("output")
	if output == "" {
		output = "openapi.yaml"
	}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	router := shared.NewOpenAPIRouter(app)

	cmdapi.RegisterAllRoutesForSpec(router)

	if err := router.WriteSchemaTo(output); err != nil {
		return fmt.Errorf("write OpenAPI spec: %w", err)
	}

	fmt.Printf("OpenAPI spec written to %s\n", output)
	return nil
}
