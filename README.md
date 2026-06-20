# transx

Wallet transfer system in Go — internal/external money transfers with an
auditable accounting ledger, event-driven processing, idempotent APIs, and
eventually consistent external settlement. See [`docs/prd.md`](docs/prd.md) for
the full product spec.

## Table of Contents

- [transx](#transx)
  - [Table of Contents](#table-of-contents)
  - [Tech Stack](#tech-stack)
  - [Repository Structure](#repository-structure)
  - [Quick Start](#quick-start)
    - [1. Start infrastructure](#1-start-infrastructure)
    - [2. Apply migrations and seed dev data](#2-apply-migrations-and-seed-dev-data)
    - [3. Run a service](#3-run-a-service)
    - [Full stack via Compose](#full-stack-via-compose)
  - [Backend CLI](#backend-cli)
  - [Common Commands](#common-commands)
  - [Overview Architecture](#overview-architecture)
  - [Wallet API](#wallet-api)
  - [Internal Transfer Flow](#internal-transfer-flow)
  - [External Transfer Flow](#external-transfer-flow)
  - [Worker Consumer Flow](#worker-consumer-flow)
  - [Idempotency](#idempotency)
  - [Backend Architecture](#backend-architecture)
  - [Key Docs](#key-docs)

## Tech Stack

| Concern        | Choice                                |
| -------------- | ------------------------------------- |
| Language       | Go 1.26                               |
| Database       | PostgreSQL 18 (native `uuidv7()`)     |
| Messaging      | Redpanda (Kafka API compatible)       |
| Gateway        | Traefik + ForwardAuth                 |
| HTTP framework | Fiber v2                              |
| DB access      | pgx v5 + sqlc-generated queries       |
| Migrations     | goose                                 |
| Provider       | pluggable `ProviderClient` (stub)     |
| Config         | viper + `.env` (env override: `A__B`) |

All identifiers use **UUID v7** (time-ordered, index-friendly).

## Repository Structure

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
docs/                       # product spec (prd.md)
plans/                      # planning artifacts and implementation phases
```

Each module under `internal/modules/<domain>/` follows a DDD split:

- `domain/` — entities and repository interfaces (no infra dependencies)
- `application/` — services (use cases) and DTOs
- `infrastructure/` — sqlc `gen/` code, `query/*.sql`, repository implementations

## Quick Start

Requires Go 1.26, Docker, and (for codegen) `sqlc` + `goose`.

### 1. Start infrastructure

```bash
docker compose up -d postgres redpanda   # Postgres + Redpanda (Kafka API)
```

Backend containers mount these local files read-only in Docker Compose:

- `backend/config.yaml` → `/app/config.yaml`
- `backend/.env` → `/app/.env`

This lets `auth` and `wallet` share the same local config and secrets while
still supporting env overrides like `POSTGRES__DATABASE_URL` and
`KAFKA__BROKERS`.

### 2. Apply migrations and seed dev data

```bash
cd backend
make migrate
make seed          # alice/bob/carol/dave/eve @transx.dev (password: password123)
                   # + wallet accounts for alice/bob (USD)
```

### 3. Run a service

```bash
make run-wallet                       # wallet: HTTP API + outbox publisher + transfer processor
go run . --config config.yaml auth    # auth service (ForwardAuth backend)
```

The wallet service needs Redpanda up — Kafka is a hard dependency and the
process fails fast at startup if the brokers or topics are missing.

External transfers go through a pluggable provider; the bundled stub is
mode-driven via `PROVIDER__MODE` (`always_success` | `always_failure` |
`always_timeout`, default `always_success`) so the full external lifecycle can
be exercised without a real provider API.

### Full stack via Compose

```bash
docker compose up -d        # traefik + auth + wallet + postgres + redpanda
```

A Traefik gateway fronts the backend on `http://localhost:4000`. Login is
public; all other routes are gated by ForwardAuth, which verifies the bearer
token and injects `X-User-Id` onto the upstream request.

```bash
# Login (public)
curl -X POST http://localhost:4000/api/v1/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@transx.dev","password":"password123"}'

# Authenticated request (Traefik verifies the token, injects X-User-Id)
curl http://localhost:4000/api/v1/accounts/<id> -H "Authorization: Bearer <token>"
```

## Backend CLI

```
transx [--config|-c config.yaml] <subcommand>

  auth      Start the auth service (POST /login + ForwardAuth /check)
  wallet    Start the wallet service (HTTP API + outbox publisher + transfer processor)
  seed      Insert development users and wallet accounts (idempotent)
  openapi-export
            Generate the merged OpenAPI spec without starting services
              --output | -o openapi.yaml   (default: openapi.yaml)
  migrate (m)  Database migrations
    up        Apply all pending migrations
    down      Rollback last migration
    status    Show migration status
```

## Common Commands

```bash
# Infrastructure
docker compose up -d postgres redpanda   # start dependencies
docker compose down                       # stop everything

# Run from backend/
make migrate        # apply goose migrations
make seed           # insert dev users + accounts
make run-wallet     # run the wallet service

# Code generation / quality
make sqlc           # regenerate sqlc query code after editing query/*.sql
make openapi        # regenerate openapi.yaml without Docker
make format         # gofmt / goimports / golines / gofumpt
make vet            # go vet ./...
make lint           # golangci-lint (enforces module boundaries via depguard)
make build          # compile the transx binary
make check          # sqlc + format + vet + lint
```

## Overview Architecture

```mermaid
flowchart TD
    FE["Client"]
    TR["Traefik\nGateway :4000"]
    AUTH["Auth Service\nFiber HTTP :4000\nJWT login + ForwardAuth check"]
    WALLET["Wallet Service\nFiber HTTP :4000\n+ outbox publisher + transfer processor + provider consumer"]
    PG[("PostgreSQL")]
    RP[("Redpanda\ntransfer.requested / provider.requested / completed / failed")]
    PROV["Payment Provider\n(stub)"]
    DLQ[("transx.wallet.dlq")]

    FE -->|"REST /api/v1"| TR
    TR -->|"/api/v1/login (public)"| AUTH
    TR -->|"ForwardAuth /api/v1/check"| AUTH
    TR -->|"all other routes + X-User-Id"| WALLET

    AUTH --> PG
    WALLET --> PG
    WALLET -->|"publish outbox events"| RP
    RP -->|"consume transfer.requested / provider.requested"| WALLET
    WALLET -->|"submit external transfer"| PROV
    WALLET -.->|"poison / exhausted retries"| DLQ
```

- **Gateway**: Traefik terminates routing and delegates authentication to the
  auth service via ForwardAuth.
- **Auth service**: issues JWTs (`POST /api/v1/login`) and verifies them for the
  gateway (`GET /api/v1/check`), echoing `X-User-Id` to upstream services.
- **Wallet service**: owns accounts, transfers, ledger entries, and the outbox
  (single consistency boundary for all money movement). Internal P2P transfers
  are processed asynchronously: the HTTP API stages a transfer plus an outbox
  event in one transaction, an outbox publisher relays events to Redpanda, and a
  transfer processor consumes them to move money atomically. External transfers
  add a reserve→submit→settle lifecycle: the processor reserves a hold, a
  provider consumer submits to the payment provider and settles the outcome
  (success debits the hold, failure releases it). All workers run as goroutines
  in the wallet binary.

## Wallet API

All routes are under `/api/v1` and gated by ForwardAuth (the gateway injects
`X-User-Id` after verifying the bearer token).

| Method | Path                      | Description                              |
| ------ | ------------------------- | ---------------------------------------- |
| `POST` | `/accounts`               | Create a wallet account for the caller   |
| `GET`  | `/accounts/{accountId}`   | Get an account balance (owner-scoped)    |
| `POST` | `/transfers`              | Create a transfer (idempotent)           |
| `GET`  | `/transfers/{transferId}` | Get a transfer (owner-scoped)            |

`POST /transfers` requires an `Idempotency-Key` header — a client-generated UUID
(uuidv7 recommended). Retrying with the same key replays the original transfer;
reusing it with a different body returns `409`. The transfer is created
`PENDING` and settled asynchronously, so poll `GET /transfers/{id}` for the
final `SUCCEEDED`/`FAILED` status.

`transferType` selects the flow. `INTERNAL` (default) moves funds to another
in-ledger account and requires `toAccountId`. `EXTERNAL` sends funds out through
the provider: omit `toAccountId` (there is no in-ledger destination) and the
`provider` is set from server config — clients never send it.

```bash
# Internal P2P transfer
curl -X POST http://localhost:4000/api/v1/transfers \
  -H "Authorization: Bearer <token>" \
  -H 'Idempotency-Key: 0190bf3e-...' \
  -H 'Content-Type: application/json' \
  -d '{"fromAccountId":"<a>","toAccountId":"<b>","amount":"100","currency":"USD","transferType":"INTERNAL"}'

# External transfer (no toAccountId)
curl -X POST http://localhost:4000/api/v1/transfers \
  -H "Authorization: Bearer <token>" \
  -H 'Idempotency-Key: 0190bf3f-...' \
  -H 'Content-Type: application/json' \
  -d '{"fromAccountId":"<a>","amount":"100","currency":"USD","transferType":"EXTERNAL"}'
```

Authorization is P2P: the `fromAccountId` must belong to the caller (otherwise
`403`); the destination may be anyone's. Reads are owner-scoped — another user's
account or transfer returns `404`. The full request/response schema is in the
generated `openapi.yaml` (`make openapi`).

## Internal Transfer Flow

```mermaid
sequenceDiagram
    actor C as Client
    participant GW as Traefik (ForwardAuth)
    participant API as Wallet API
    participant DB as PostgreSQL
    participant PUB as Outbox Publisher
    participant RP as Redpanda
    participant PR as Transfer Processor

    C->>GW: POST /transfers (Bearer, Idempotency-Key)
    GW->>API: forward + X-User-Id
    Note over API: validate, authorize from-account,<br/>check idempotency key
    API->>DB: BEGIN — INSERT transfer(PENDING)<br/>+ INSERT outbox(transfer.requested) — COMMIT
    API-->>C: 202 { transferId, status: PENDING }

    loop poll outbox
        PUB->>DB: SELECT pending outbox events
        PUB->>RP: publish transfer.requested
        PUB->>DB: mark PUBLISHED
    end

    PR->>RP: consume transfer.requested
    Note over PR: dedup via inbox_events
    PR->>DB: BEGIN — lock accounts (ordered)<br/>debit from / credit to (if ACTIVE & funded)<br/>write ledger, status=SUCCEEDED<br/>+ outbox(transfer.completed) — COMMIT
    Note over PR,DB: insufficient funds / not active →<br/>status=FAILED + outbox(transfer.failed)
    PR->>RP: commit offset

    C->>API: GET /transfers/{id} (poll)
    API-->>C: status SUCCEEDED / FAILED
```

## External Transfer Flow

An external transfer leaves the system through a provider, so it cannot settle
in one transaction. It splits into reserve (hold funds) and settle (after the
provider responds), each its own transaction guarded by transfer status.

```mermaid
sequenceDiagram
    actor C as Client
    participant API as Wallet API
    participant DB as PostgreSQL
    participant PR as Transfer Processor
    participant PC as Provider Consumer
    participant PV as Payment Provider

    C->>API: POST /transfers (EXTERNAL, no toAccountId)
    API->>DB: INSERT transfer(PENDING) + outbox(transfer.requested)
    API-->>C: 202 { transferId, status: PENDING }

    PR->>DB: BEGIN — guard PENDING<br/>available → hold, ledger HOLD<br/>status=RESERVED + outbox(provider.requested) — COMMIT

    PC->>PV: Submit(transferId, amount, currency)
    alt provider SUCCESS
        PV-->>PC: { SUCCESS, referenceId }
        PC->>DB: BEGIN — guard RESERVED<br/>debit hold, ledger DEBIT<br/>status=SUCCEEDED + outbox(transfer.completed) — COMMIT
    else provider FAILURE
        PV-->>PC: { FAILURE, reason }
        PC->>DB: BEGIN — guard RESERVED<br/>release hold → available, ledger RELEASE<br/>status=FAILED + outbox(transfer.failed) — COMMIT
    else provider timeout (transient)
        PV-->>PC: error
        Note over PC: escalate retry tiers then DLQ,<br/>transfer stays RESERVED
    end

    C->>API: GET /transfers/{id} (poll)
    API-->>C: status SUCCEEDED / FAILED
```

## Worker Consumer Flow

Several background workers run as goroutines in the wallet binary, supervised by
an errgroup so a fatal worker error brings the process down for a clean restart.

```mermaid
flowchart TD
    DB[("PostgreSQL")]
    RPREQ[("transfer.requested")]
    RPPROV[("transfer.provider.requested")]
    RPDONE[("transfer.completed")]
    RPFAIL[("transfer.failed")]
    PROV["Payment Provider\n(stub)"]
    RETRY[("transx.wallet.retry-6s / 30s / 5m")]
    DLQ[("transx.wallet.dlq")]

    subgraph Publisher["Outbox Publisher (single owner)"]
        POLL["poll outbox_events\nstatus=PENDING, FIFO"]
        MARK["mark PUBLISHED\nWHERE status=PENDING"]
    end

    subgraph Processor["Transfer Processor — group: wallet-processor"]
        ROUTE{"transfer_type?"}
        EXEC["ExecuteInternalTransfer\nlock accounts (ORDER BY id)\nconditional debit / credit\nledger + status + outbox"]
        RESV["ReserveExternalTransfer\nhold funds, ledger HOLD\nstatus=RESERVED + outbox"]
        CLASS{"error?"}
    end

    subgraph Provider["Provider Consumer — group: wallet-provider"]
        SUBMIT["client.Submit"]
        SETTLE["SettleExternalTransfer\nsuccess: debit hold (DEBIT)\nfailure: release hold (RELEASE)"]
        PCLASS{"error?"}
    end

    subgraph Retry["Retry-tier consumers"]
        HOLD["HoldUntil(retryAt)\nrepublish to source topic"]
    end

    POLL -->|"transfer.requested / provider.requested"| RPREQ & RPPROV
    POLL -.-> MARK
    MARK --> DB

    RPREQ --> ROUTE
    ROUTE -->|INTERNAL| EXEC
    ROUTE -->|EXTERNAL| RESV
    EXEC --> DB
    RESV --> DB
    RESV -->|provider.requested| RPPROV
    EXEC -->|completed / failed| RPDONE & RPFAIL
    EXEC --> CLASS
    CLASS -->|transient| RETRY
    CLASS -->|permanent / poison| DLQ

    RPPROV --> SUBMIT
    SUBMIT --> PROV
    SUBMIT --> SETTLE
    SETTLE --> DB
    SETTLE -->|completed / failed| RPDONE & RPFAIL
    SUBMIT --> PCLASS
    PCLASS -->|timeout / transient| RETRY
    PCLASS -->|poison| DLQ
    RETRY --> HOLD
    HOLD --> RPREQ & RPPROV
```

- **Outbox publisher** drains `outbox_events` in FIFO order and marks each
  `PUBLISHED` only after a successful publish (`WHERE status='PENDING'` guards
  against double-marking). A single publisher owns the table.
- **Transfer processor** (group `wallet-processor`) deduplicates via
  `inbox_events`, then routes by `transfer_type` read from the database.
  `INTERNAL` moves money in one transaction: it locks both accounts in a
  deterministic order (avoids cross deadlock), runs a conditional debit
  (`available_balance >= amount AND status='ACTIVE'`) and credit, writes the
  ledger, advances status, and stages the completion event — all atomically.
  `EXTERNAL` reserves a hold (`available → hold`, ledger `HOLD`), sets status
  `RESERVED`, and stages `transfer.provider.requested`.
- **Provider consumer** (group `wallet-provider`) consumes
  `transfer.provider.requested`, submits to the payment provider, and settles in
  one transaction: success debits the hold (ledger `DEBIT`, status `SUCCEEDED`),
  business failure releases it (ledger `RELEASE`, status `FAILED`). A provider
  timeout is treated as transient and retried through the tiers. Each settle step
  is guarded by the `RESERVED` status so a redelivery never double-settles.
- **Retries**: transient failures (serialization, deadlock, provider timeout)
  escalate through delayed-retry tiers (`6s` → `30s` → `5m`); poison messages and
  exhausted retries go to `transx.wallet.dlq`, so one bad message never wedges the
  partition. A timed-out external transfer stays `RESERVED` until it lands in the
  DLQ (provider reconciliation is out of scope for now).

## Idempotency

Two independent layers protect against duplicate money movement:

| Layer              | Mechanism                                                                                                                                                                                   | Location                                                                |
| ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| **API**            | Unique index `(user_id, idempotency_key)` + `request_hash` — same key & body replays the original transfer, different body returns `409`                                                    | `wallet/application/services/transfer_service.go`                       |
| **Kafka consumer** | `inbox_events` keyed on `(consumer_group, message_key)` — a redelivered message is skipped; the per-step status guard (`PENDING` for reserve/internal, `RESERVED` for settle) inside each transaction is the final double-spend defense. The two consumer groups (`wallet-processor`, `wallet-provider`) dedup in separate namespaces. | `wallet/infrastructure/processor`, `wallet/infrastructure/repositories` |

```mermaid
flowchart LR
    subgraph API["API Layer"]
        A1["POST /transfers"]
        A2{"FindByUserAndKey\nexists?"}
        A3["replay (body matches)\nor 409 (body differs)"]
        A4["INSERT transfer(PENDING)\n+ outbox"]
        A1 --> A2
        A2 -->|yes| A3
        A2 -->|no| A4
    end

    subgraph Consumer["Kafka Consumer Layer"]
        K1["consume transfer.requested"]
        K2{"inbox_events\nprocessed?"}
        K3["skip"]
        K4{"transfer status\n= PENDING?"}
        K5["move money + markProcessed"]
        K6["no-op (already settled)"]
        K1 --> K2
        K2 -->|yes| K3
        K2 -->|no| K4
        K4 -->|yes| K5
        K4 -->|no| K6
    end
```

## Backend Architecture

Clean architecture by domain module:

```
internal/
├── modules/
│   ├── auth/       POST /login, ForwardAuth /check (JWT)
│   └── wallet/     accounts, transfers, ledger, outbox + transfer processor + provider consumer
├── platform/
│   ├── config/     viper YAML config (env override SECTION__KEY)
│   ├── postgres/   pgxpool connection + WithTx helper
│   ├── kafka/      Producer + Consumer (manual commit, delayed-retry holds)
│   ├── httpserver/ Fiber server (/healthz, /readyz) + struct validator
│   ├── logger/     structured slog with color support
│   └── middleware/ RequestID, UserID (X-User-Id from ForwardAuth)
├── common/
│   ├── apperror/   AppError (carries HTTP status)
│   └── kafkatopic/ topic names, event types, retry-tier definitions
└── shared/         lifecycle, pgconv
cmd/
├── api/handlers/   HTTP handlers (transport layer)
├── api/routes.go   RegisterRoutes (auth) / RegisterWalletRoutes / RegisterAllRoutesForSpec
└── shared/         OpenAPI-aware route generator
cli/                CLI entry points (auth | wallet | seed | migrate | openapi-export)
```

Modules use `application/dto` for transport-facing commands and responses,
`application/services` for business logic, `domain/entities` for
transport-agnostic domain objects, and `domain/interfaces` for ports and
repositories. Infrastructure implements those interfaces over sqlc-generated
queries.

Each service registers only its own routes — auth runs `RegisterRoutes`, wallet
runs `RegisterWalletRoutes` — so neither binary carries the other's handlers.
The OpenAPI exporter combines both groups with nil handlers
(`RegisterAllRoutesForSpec`) into a single merged `openapi.yaml`.

Conventions:

- **IDs are UUID v7** — DB columns default to `uuidv7()` (Postgres 18); let the
  DB assign them.
- **Money is `decimal.Decimal`** mapped to `NUMERIC(20,4)`; never floats.
- **Errors** return `*apperror.AppError` (carries HTTP status); `DomainErrorHandler`
  maps them to responses.
- **Config**: add fields to `internal/platform/config/config.go`; env override
  format is `SECTION__KEY` (e.g. `AUTH__JWT_SECRET`, `PROVIDER__MODE`). Secrets
  stay in `.env`.
- **Money never settles across a network call in one tx** — external transfers
  reserve a hold first, then settle in a second transaction after the provider
  responds, so a mid-flight failure leaves funds held rather than lost.

## Key Docs

- Product requirements: `docs/prd.md`
- OpenAPI spec: `backend/openapi.yaml`

```

```
