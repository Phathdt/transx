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
			{Name: "auth", Usage: "Start standalone auth service (ForwardAuth backend)", Action: mycli.RunAuthService},
			{
				Name:   "wallet",
				Usage:  "Start the wallet HTTP API (API only; workers run separately)",
				Action: mycli.RunWalletService,
			},
			{
				Name:   "transfer",
				Usage:  "Start the transfer HTTP API (API only; workers run separately)",
				Action: mycli.RunTransferService,
			},
			{
				Name:   "consumer",
				Usage:  "Kafka→Temporal bridge for transfer.requested (+ start retries)",
				Action: mycli.RunConsumer,
			},
			{
				Name:   "notification",
				Usage:  "Consume terminal transfer events and dispatch notifications",
				Action: mycli.RunNotificationService,
			},
			{
				Name:   "fx",
				Usage:  "Run the FX service (gRPC Quote + QuoteFee)",
				Action: mycli.RunFXService,
			},
			{
				Name:   "wallet-grpc",
				Usage:  "Run the Wallet gRPC service (Move/Hold/SettleHold/ReleaseHold)",
				Action: mycli.RunWalletGRPCService,
			},
			{
				Name:   "bank-grpc",
				Usage:  "Run the Bank gRPC service (Submit/Query, mode-driven, stateless)",
				Action: mycli.RunBankGRPCService,
			},
			{
				Name:   "transfer-worker",
				Usage:  "Run the Transfer service's Temporal worker (TransferWorkflow + activities)",
				Action: mycli.RunTransferWorker,
			},
			{Name: "seed", Usage: "Insert development users (idempotent)", Action: mycli.Seed},
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
