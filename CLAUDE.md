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
make check          # sqlc + format + vet + lint — run before considering work done
make sqlc           # regenerate query code after editing internal/modules/*/infrastructure/query/*.sql
make migrate        # apply goose migrations
make seed           # insert dev users (idempotent)
go run . --config config.yaml auth     # run auth service
go run . --config config.yaml wallet   # run wallet service
```

There is no unit-test suite yet; verify by building and exercising endpoints
with `curl` against a running service (Postgres must be up via `docker compose`).

## Architecture conventions

- **Service runners** live in `cli/` (`runAuth`, `runWallet`). Pattern: load
  config → init logger → connect Postgres eagerly → build module wiring → start
  `httpserver` → block on signal/errgroup. Mirror an existing runner.
- **DDD modules** under `internal/modules/<domain>/`:
  - `domain/entities`, `domain/interfaces` — no infra imports.
  - `application/services`, `application/dto` — use cases.
  - `infrastructure/repositories` — implement domain interfaces using sqlc
    `gen/` code; `infrastructure/query/*.sql` is the sqlc source.
- **Platform** (`internal/platform/`) is shared infra: `config`, `postgres`,
  `kafka`, `httpserver` (Fiber, serves `/healthz` + `/readyz`), `logger`,
  `middleware`. Reuse it; do not hand-roll HTTP servers.
- **HTTP routes** register in `cmd/api/routes.go` via the oaswrap spec router so
  they appear in the exported OpenAPI spec. Handlers in `cmd/api/handlers/`.
  Errors return `*apperror.AppError` (carries HTTP status); `DomainErrorHandler`
  maps them.

## Rules

- **IDs are UUID v7.** DB columns default to `uuidv7()` (Postgres 18); let the
  DB assign them. Don't hardcode IDs in seeds.
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
