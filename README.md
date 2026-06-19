# transx

Wallet transfer system — internal/external money transfers with an auditable
accounting ledger, event-driven processing, idempotent APIs, and eventually
consistent external settlement. See [`docs/prd.md`](docs/prd.md) for the full
product spec.

## Architecture

```
Client → Traefik (gateway) → ForwardAuth (auth service) → Wallet service → PostgreSQL
                                                              ↓
                                                    Redpanda (Kafka API)
                                                              ↓
                                            Notification / Analytics consumers
```

- **Gateway**: Traefik terminates TLS, routes, and delegates authentication to
  the auth service via ForwardAuth.
- **Auth service**: issues JWTs (`POST /api/v1/login`) and verifies them for the
  gateway (`GET /api/v1/check`), echoing `X-User-Id` to upstream services.
- **Wallet service**: owns accounts, transfers, ledger entries, and the outbox
  (single consistency boundary for all money movement).

## Tech stack

| Concern        | Choice                                  |
| -------------- | --------------------------------------- |
| Language       | Go 1.26                                 |
| Database       | PostgreSQL 18 (native `uuidv7()`)       |
| Messaging      | Redpanda (Kafka API compatible)         |
| Gateway        | Traefik + ForwardAuth                   |
| HTTP framework | Fiber v2                                |
| DB access      | pgx v5 + sqlc-generated queries         |
| Migrations     | goose                                   |
| Config         | viper + `.env` (env override: `A__B`)   |

All identifiers use **UUID v7** (time-ordered, index-friendly).

## Layout

```
backend/
├── main.go                 # urfave/cli entrypoint; one subcommand per service
├── cli/                    # service runners (auth, wallet), migrate, seed
├── cmd/
│   ├── api/                # HTTP handlers + OpenAPI route registration
│   └── shared/             # OpenAPI router factory
├── internal/
│   ├── modules/<domain>/   # DDD per module: domain / application / infrastructure
│   ├── platform/           # config, postgres, kafka, httpserver, logger, middleware
│   ├── common/             # apperror, kafkatopic
│   └── shared/             # lifecycle, pgconv
└── db/migrations/          # goose SQL migrations
```

Each module follows a DDD split:

- `domain/` — entities and repository interfaces (no infra dependencies)
- `application/` — services (use cases) and DTOs
- `infrastructure/` — sqlc `gen/` code, `query/*.sql`, repository implementations

## Getting started

Requires Go 1.26, Docker, and (for codegen) `sqlc` + `goose`.

```bash
# 1. Start infrastructure (Postgres, Redpanda, Traefik)
docker compose up -d postgres redpanda

# 2. Apply migrations and seed dev users
cd backend
make migrate
make seed          # alice/bob/carol/dave/eve @transx.dev, password: password123

# 3. Run a service
make run-wallet    # wallet service (health endpoints only, for now)
go run . --config config.yaml auth   # auth service
```

### Full stack via Compose

```bash
docker compose up -d        # traefik + auth + wallet + postgres + redpanda
```

The gateway listens on host port `4000`. Login is public; all other routes are
gated by ForwardAuth.

```bash
# Login (public)
curl -X POST http://localhost:4000/api/v1/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@transx.dev","password":"password123"}'

# Authenticated request (Traefik verifies the token, injects X-User-Id)
curl http://localhost:4000/healthz -H "Authorization: Bearer <token>"
```

## Make targets

| Target         | Description                            |
| -------------- | -------------------------------------- |
| `make sqlc`    | Generate sqlc code                     |
| `make format`  | Format Go code (gofmt/goimports/gofumpt) |
| `make vet`     | Run `go vet`                           |
| `make lint`    | Run golangci-lint                      |
| `make build`   | Build the `transx` binary              |
| `make check`   | sqlc + format + vet + lint             |
| `make openapi` | Generate `openapi.yaml`                |
| `make migrate` | Apply database migrations              |
| `make seed`    | Insert development users               |
| `make run-wallet` | Run the wallet service              |
