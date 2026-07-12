# CLAUDE.md

Guidance for AI agents working in this repository. Keep changes consistent with
the patterns below.

## Project

transx — a wallet transfer system in Go. Product spec: `docs/prd.md`. Each
backend service is a subcommand of one binary (`backend/main.go`, urfave/cli).

## Commands

Run from `backend/`:

```bash
make build          # compile (run after code changes)
make check          # sqlc + format + vet + lint + test + coverage — run before considering work done
make sqlc           # regenerate query code after editing internal/modules/*/infrastructure/query/*.sql
make proto          # regenerate gRPC code after editing proto/*.proto (buf)
make mock           # regenerate mockery mocks into internal/testmocks
make test           # unit tests (go test -short -p 1 ./...)
make test-integration  # tagged integration tests (requires Docker)
make coverage       # module + worker coverage gate (>= 90%)
make migrate        # apply goose migrations
make seed           # insert dev users (idempotent)
go run . --config config.yaml auth            # auth service (ForwardAuth backend)
go run . --config config.yaml wallet          # wallet HTTP API (API only)
go run . --config config.yaml transfer        # transfer HTTP API (API only)
go run . --config config.yaml consumer        # transfer processor + provider + retries
go run . --config config.yaml notification    # terminal transfer event notifications
go run . --config config.yaml stub-provider   # stub payment provider (POST /submit)
go run . --config config.yaml fx              # FX service (gRPC Quote + QuoteFee)
```

The wallet workload is split across independent commands on the one `transx`
binary so each scales/deploys separately: `wallet` serves only the `/accounts`
HTTP routes, `transfer` serves only the `/transfers` HTTP routes (both are
thin process/HTTP wiring over the same `wallet` DDD module — there is no
separate transfer module or schema), the background work lives in
`consumer` (processes the transfer lifecycle + retries) and `notification`
(consumes terminal transfer events and records notification audit rows). Outbox
events drain to Kafka via the external `iris` CDC service (Postgres logical
replication), which must run single-instance to preserve FIFO ordering.
`consumer` reaches the payment provider over HTTP via `stub-provider`. FX
quoting (rates + fees) lives in the standalone `fx` service, which `consumer`
reaches over gRPC.

Tests live beside the code they cover (`*_test.go`). Unit tests run with
`make test`; integration tests are behind the `integration` build tag
(`make test-integration`, needs Postgres/Kafka via `docker compose`). Mocks are
mockery-generated into `internal/testmocks` (`make mock`). `make check` runs the
full gate (sqlc + format + vet + lint + test + coverage); module and worker
coverage must stay >= 90%.

## Architecture conventions

- **Service runners** live in `cli/` (`runAuth`, `runWallet`, `runTransfer`,
  `runConsumer`, `runNotificationService`, `runStubProvider`, `runFXService`).
  Each runner is self-contained: load config → init logger → connect Postgres
  eagerly → build module wiring → start `httpserver`/gRPC and/or workers → block
  on signal/errgroup. Mirror an existing runner.
- **DDD modules** under `internal/modules/<domain>/`:
  - `domain/entities`, `domain/interfaces` — no infra imports.
  - `application/services`, `application/dto` — use cases.
  - `infrastructure/repositories` — implement domain interfaces using sqlc
    `gen/` code; `infrastructure/query/*.sql` is the sqlc source.
  - `infrastructure/provider` — the external payment-provider HTTP client (a
    domain-port adapter) plus the shared wire contract and the stub server's
    handler. Client and server share `http_contract.go` so they cannot drift.
- **Worker logic** lives under `cmd/<worker>/` (`cmd/consumer` — transfer
  processor, provider consumer, retry tiers). These are Kafka consume/drain
  orchestration loops, not domain adapters, so they sit beside `cmd/api` rather
  than inside a module. They import a module's `domain/interfaces` and are wired
  up by the matching `cli/` runner.
- **gRPC handlers** live under `cmd/grpc/` (`fx_grpc_handler.go` — the FX
  server adapter). They translate between the generated proto types and a
  module's application service. The `fx` module owns rate/fee logic; the wallet
  module reaches it through a gRPC client adapter
  (`wallet/infrastructure/fx/grpc_fx_client.go`) implementing `FXService`.
- **Protos** live in `proto/`; `make proto` (buf) regenerates Go code into
  `internal/platform/grpc/gen/`. Reuse `platform/grpc.Serve` to run a server;
  do not hand-roll the gRPC lifecycle.
- **Platform** (`internal/platform/`) is shared infra: `config`, `postgres`,
  `kafka`, `httpserver` (Fiber, serves `/healthz` + `/readyz`), `grpc`, `logger`,
  `middleware`. Reuse it; do not hand-roll HTTP or gRPC servers.
- **HTTP routes** register in `cmd/api/routes.go` via the oaswrap spec router so
  they appear in the exported OpenAPI spec. Handlers in `cmd/api/handlers/`.
  Errors return `*apperror.AppError` (carries HTTP status); `DomainErrorHandler`
  maps them.

## Rules

- **IDs are UUID v7.** DB columns default to `uuidv7()` (Postgres 18); let the
  DB assign them. Don't hardcode IDs in seeds.
- **FX fees** are a flat amount per source currency in config (`fx.fees`, keyed
  by source currency code), charged on internal transfers that convert out of the
  source currency as a third `FEE` ledger entry. A missing/non-positive entry =
  no fee. Not a percentage; don't reintroduce a rate-based fee.
- **Config**: add fields to `internal/platform/config/config.go`. Env override
  format is `SECTION__KEY` (e.g. `AUTH__JWT_SECRET`). Secrets stay in `.env` /
  env vars, never committed.
- **sqlc**: after changing a migration schema or `query/*.sql`, run `make sqlc`.
  A module's sqlc block stays commented in `sqlc.yaml` until its `query/*.sql`
  exists (sqlc fails on empty query globs).
- **Migrations** are goose files in `db/migrations/`. Keep seed data out of
  migrations — use the `seed` command.
- Match the surrounding code's style. Run `make check` before finishing.
- Code comments explain _why_, not plan/phase references.
