package main

import (
	"log"
	"os"

	mycli "transx/cli"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "transx",
		Usage: "transx backend services",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Value:   "config.yaml",
				Usage:   "Path to configuration file",
			},
		},
		Commands: []*cli.Command{
			{Name: "wallet", Usage: "Start standalone wallet service", Action: mycli.RunWalletService},
			{
				Name:  "openapi-export",
				Usage: "Generate OpenAPI spec without starting services",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "output",
						Aliases: []string{"o"},
						Value:   "openapi.yaml",
						Usage:   "Path to write the generated OpenAPI YAML",
					},
				},
				Action: mycli.RunOpenAPIExport,
			},
			{
				Name:    "migrate",
				Aliases: []string{"m"},
				Usage:   "Database migrations",
				Subcommands: []*cli.Command{
					{Name: "up", Usage: "Apply all pending migrations", Action: mycli.MigrateUp},
					{Name: "down", Usage: "Rollback last migration", Action: mycli.MigrateDown},
					{Name: "status", Usage: "Show migration status", Action: mycli.MigrateStatus},
				},
			},
		},
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
