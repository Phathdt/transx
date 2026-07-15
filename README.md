# transx

Wallet transfer system in Go — internal/external money transfers with an
auditable accounting ledger, Temporal saga orchestration, idempotent APIs, and
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
    - [Auth BFF \(web\)](#auth-bff-web)
  - [Wallet API](#wallet-api)
  - [Internal Transfer Flow](#internal-transfer-flow)
  - [External Transfer Flow](#external-transfer-flow)
  - [Multi-Currency \& FX Settlement](#multi-currency--fx-settlement)
    - [Internal transfers (cross-currency capable)](#internal-transfers-cross-currency-capable)
    - [External transfers (single-currency only)](#external-transfers-single-currency-only)
  - [Worker / Temporal Flow](#worker--temporal-flow)
  - [Idempotency](#idempotency)
  - [Backend Architecture](#backend-architecture)
  - [Key Docs](#key-docs)

## Tech Stack

| Concern        | Choice                                  |
| -------------- | --------------------------------------- |
| Language       | Go 1.26                                 |
| Frontend       | React Router v7 framework (SSR) + React 19 |
| Database       | PostgreSQL 18 (native `uuidv7()`)       |
| Session store  | Redis (auth refresh tokens)             |
| Messaging      | Kafka (Redpanda only in local demo)     |
| Orchestration  | Temporal (TransferWorkflow saga)        |
| Gateway        | Traefik + ForwardAuth                   |
| HTTP framework | Fiber v2                                |
| DB access      | pgx v5 + sqlc-generated queries         |
| Migrations     | goose                                   |
| External bank  | Bank gRPC (mode-driven fake)            |
| FX             | standalone gRPC service (buf-generated) |
| Config         | viper + `.env` (env override: `A__B`)   |

All identifiers use **UUID v7** (time-ordered, index-friendly).

## Repository Structure

```
backend/
├── main.go                 # urfave/cli entrypoint; one subcommand per service
├── cli/                    # service runners (auth, wallet, transfer, consumer,
│                           #   notification, fx, wallet-grpc, bank-grpc,
│                           #   transfer-worker), migrate, seed
├── proto/                  # gRPC service definitions (buf source)
├── cmd/
│   ├── api/                # HTTP handlers + OpenAPI route registration
│   ├── consumer/           # Kafka→Temporal bridge for transfer.requested
│   ├── worker/             # TransferWorkflow + Temporal activities
│   ├── grpc/               # gRPC handler adapters (fx, wallet, bank)
│   └── shared/             # OpenAPI router factory
├── internal/
│   ├── modules/<domain>/   # DDD per module: domain / application / infrastructure
│   ├── platform/           # config, postgres, redis, kafka, httpserver, grpc, logger
│   ├── common/             # apperror, kafkatopic, provider (Bank fake)
│   └── shared/             # lifecycle, pgconv
└── db/migrations/          # goose SQL migrations
frontend/                   # React Router v7 framework app (Auth BFF + UI)
docs/                       # product + architecture docs
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
# Local demo stack: Redpanda stands in for Kafka (not a production choice).
# Redis is required for auth refresh sessions.
docker compose up -d postgres redis redpanda temporal temporal-postgres
```

Backend containers mount these local files read-only in Docker Compose:

- `backend/config.yaml` → `/app/config.yaml`
- `backend/.env` → `/app/.env`

This lets services share the same local config and secrets while still
supporting env overrides like `POSTGRES__DATABASE_URL`, `KAFKA__BROKERS`, and
`TEMPORAL__HOST_PORT`.

### 2. Apply migrations and seed dev data

```bash
cd backend
make migrate
make seed          # alice/bob/carol/dave/eve @transx.dev (password: password123)
                   # + wallet accounts for alice/bob (USD)
```

### 3. Run a service

The workload is split across independent commands on the one `transx` binary so
each scales and deploys separately:

```bash
make run-wallet            # wallet: HTTP API only
make run-consumer          # consumer: Kafka→Temporal bridge
make run-notification      # notification: terminal transfer events
make run-fx                # fx: Quote + QuoteFee (gRPC)
make run-wallet-grpc       # wallet money RPCs
make run-bank-grpc         # bank Submit/Query (mode-driven)
make run-transfer-worker   # Temporal TransferWorkflow + activities
go run . --config config.yaml auth
go run . --config config.yaml transfer
```

`consumer`, `notification`, and `transfer-worker` need Kafka/Temporal as
applicable — they fail fast at startup if hard dependencies are missing.
`wallet`/`transfer` (API only) and `auth` do not start workflows themselves.
Outbox events are drained to Kafka via the external `iris` CDC service (Postgres
logical replication, single-instance for FIFO ordering). Local compose uses
**Redpanda only as a demo Kafka broker**, not as the production messaging product.

External transfers go through **Bank gRPC** (`bank-grpc`), mode-driven via
`BANK__MODE` (`always_success` | `always_failure` | `always_timeout`). The Temporal
worker dials it at `BANK__GRPC_ADDRESS`.

FX quoting lives in the standalone `fx` service (`FX__GRPC_ADDRESS`), dialed by
the transfer worker during prepare activities.

### Full stack via Compose

```bash
docker compose up -d
# traefik + auth + wallet + transfer + consumer + notification + fx
# + wallet-grpc + bank-grpc + transfer-worker + temporal (+ ui)
# + iris + postgres + redis + redpanda (Kafka-compatible demo broker only)
```

A Traefik gateway fronts the backend on `http://localhost:4000`. Auth token
endpoints (`/login`, `/refresh`, `/logout`, `/session`) are public JSON APIs;
wallet/transfer/inbox routes are gated by ForwardAuth (Bearer access token →
`X-User-Id`).

**Web UI auth (Auth BFF):** React Router Node owns the HttpOnly refresh cookie.
Browser → RR (`/api/auth/*`) → Go auth (JSON AT+RT). Domain API calls still use
Bearer access tokens against Traefik.

```bash
# Login against Go auth (returns accessToken + refreshToken JSON — no Set-Cookie)
curl -X POST http://localhost:4000/api/v1/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@transx.dev","password":"password123"}'

# Authenticated request (Traefik verifies the access token, injects X-User-Id)
curl http://localhost:4000/api/v1/accounts/<accountRef> -H "Authorization: Bearer <accessToken>"

# Frontend (RR framework + Auth BFF cookie)
cd frontend && cp -n .env.example .env && yarn install && yarn dev  # :3000
```

## Backend CLI

```
transx [--config|-c config.yaml] <subcommand>

  auth             Start the auth service (JSON AT+RT + ForwardAuth /check)
  wallet           Start the wallet HTTP API (API only)
  transfer         Start the transfer HTTP API (API only)
  consumer         Kafka→Temporal bridge for transfer.requested (+ start retries)
  notification     Consume terminal transfer events and dispatch notifications
  fx               Run the FX service (gRPC Quote + QuoteFee)
  wallet-grpc      Run Wallet gRPC (Move/Hold/SettleHold/ReleaseHold)
  bank-grpc        Run Bank gRPC (Submit/Query, mode-driven)
  transfer-worker  Run Temporal TransferWorkflow + activities
  seed             Insert development users and wallet accounts (idempotent)
  openapi-export   Generate the merged OpenAPI spec without starting services
                     --output | -o openapi.yaml   (default: openapi.yaml)
  migrate (m)      Database migrations
    up             Apply all pending migrations
    down           Rollback last migration
    status         Show migration status
```

## Common Commands

```bash
# Infrastructure
# Redpanda = local Kafka stand-in for demos only; Redis = auth refresh sessions
docker compose up -d postgres redis redpanda temporal temporal-postgres
docker compose down

# Run from backend/
make migrate
make seed
make run-wallet
make run-consumer
make run-notification
make run-fx
make run-wallet-grpc
make run-bank-grpc
make run-transfer-worker

# Code generation / quality
make sqlc
make proto
make mock
make openapi
make format
make vet
make lint
make test
make test-integration
make coverage
make build
make check
```

## Overview Architecture

```mermaid
flowchart TD
    BR["Browser"]
    RR["React Router Node\n:3000 SSR + Auth BFF"]
    TR["Traefik\nGateway :4000"]
    AUTH["Auth Service\nJSON AT+RT + /check"]
    REDIS[("Redis\nrefresh sessions")]
    WALLET["Wallet API\n/accounts"]
    TRANSFER["Transfer API\n/transfers"]
    IRIS["iris CDC\nPostgres → Kafka"]
    BRIDGE["consumer\nKafka→Temporal bridge"]
    TEMP["Temporal Server"]
    WORKER["transfer-worker\nTransferWorkflow"]
    NOTIF["notification\nterminal events"]
    WGRPC["wallet-grpc\nMove/Hold/Settle/Release"]
    BGRPC["bank-grpc\nSubmit/Query"]
    FX["fx gRPC\nQuote + QuoteFee"]
    PG[("PostgreSQL")]
    KREQ[("Kafka\ntransfer.requested")]
    KOK[("Kafka\ntransfer.completed")]
    KFAIL[("Kafka\ntransfer.failed")]
    TDLQ[("transx.transfer.dlq")]
    NDLQ[("transx.notification.dlq")]

    BR -->|"HTML / same-origin\n/api/auth/*"| RR
    RR -->|"HttpOnly RT cookie\non FE host"| BR
    RR -->|"JSON login/refresh/logout/session"| AUTH
    AUTH --> REDIS
    AUTH --> PG

    BR -->|"Bearer access token\nREST /api/v1 domain"| TR
    TR -->|"/login|/refresh|/logout|/session\n(public JSON)"| AUTH
    TR -->|"ForwardAuth /check\nBearer AT"| AUTH
    TR -->|"/accounts* + X-User-Id"| WALLET
    TR -->|"/transfers* + X-User-Id"| TRANSFER
    TR -->|"/inbox* + X-User-Id"| NOTIF

    WALLET --> PG
    TRANSFER -->|"PENDING + outbox"| PG
    IRIS -->|"logical replication"| PG
    IRIS -->|"event_type topic"| KREQ
    IRIS -->|"event_type topic"| KOK
    IRIS -->|"event_type topic"| KFAIL
    KREQ --> BRIDGE
    BRIDGE -->|"StartWorkflow transfer-{id}"| TEMP
    TEMP --> WORKER
    WORKER --> WGRPC
    WORKER --> BGRPC
    WORKER --> FX
    WORKER -->|"MarkTerminal status+outbox"| PG
    WGRPC --> PG
    KOK --> NOTIF
    KFAIL --> NOTIF
    NOTIF --> PG
    BRIDGE -.->|"poison / start retries exhausted"| TDLQ
    NOTIF -.->|"poison / exhausted retries"| NDLQ
```

### Auth BFF (web)

```mermaid
sequenceDiagram
    actor B as Browser
    participant RR as React Router Node
    participant A as Go Auth
    participant R as Redis
    participant T as Traefik / APIs

    B->>RR: POST /api/auth/login {email,password}
    RR->>A: POST /api/v1/login
    A->>R: store RT hash
    A-->>RR: {accessToken, refreshToken}
    RR-->>B: Set-Cookie HttpOnly RT + JSON AT
    Note over B: AT in memory only

    B->>RR: document /app/* (cookie RT)
    RR->>A: POST /api/v1/session {refreshToken}
    A->>R: validate (no rotate)
    A-->>RR: 204
    RR-->>B: HTML shell

    B->>RR: POST /api/auth/refresh (cookie)
    RR->>A: POST /api/v1/refresh {refreshToken}
    A->>R: rotate RT
    A-->>RR: {accessToken, refreshToken}
    RR-->>B: new cookie + AT JSON

    B->>T: GET /api/v1/accounts Authorization Bearer AT
    T->>A: ForwardAuth GET /check
    A-->>T: 200 + X-User-Id
    T-->>B: domain response
```

- **React Router Node**: SSR UI + Auth BFF. Owns the HttpOnly refresh cookie on
  the FE host. Proxies login/refresh/logout/session to Go as JSON only.
- **Gateway**: Traefik routes domain APIs; ForwardAuth verifies **access** JWT
  and injects `X-User-Id`.
- **Auth**: issues short-lived access JWTs + opaque refresh tokens (Redis). No
  browser cookies on the auth service itself.
- **Transfer API**: stages `PENDING` transfer + `transfer.requested` outbox in
  one transaction; does not move money.
- **iris**: CDC drain of outbox → Kafka topics named by `event_type` (single-instance FIFO). Local demo compose uses Redpanda as a Kafka-compatible broker only.
- **consumer**: only the Kafka→Temporal bridge (inbox dedup + `ExecuteWorkflow`).
- **transfer-worker**: runs the Temporal saga (INTERNAL/EXTERNAL activities).
- **wallet-grpc / bank-grpc / fx**: money, bank outcomes, and FX quotes over gRPC.
- **notification**: terminal events → notification audit rows (+ inbox HTTP).

## Wallet API

All routes are under `/api/v1` and gated by ForwardAuth (the gateway injects
`X-User-Id` after verifying the bearer token).

| Method | Path                                   | Description                                                        |
| ------ | -------------------------------------- | ------------------------------------------------------------------ |
| `POST` | `/accounts`                            | Create a wallet account for the caller                             |
| `GET`  | `/accounts`                            | List the caller's accounts (paginated; currency/status filters)    |
| `GET`  | `/accounts/{accountRef}`               | Get an account balance (owner-scoped)                              |
| `GET`  | `/accounts/{accountType}/{accountRef}` | Look up internal/external beneficiary account info                 |
| `POST` | `/transfers`                           | Create a transfer (idempotent)                                     |
| `GET`  | `/transfers`                           | List the caller's transfers (paginated; status/accountRef filters) |
| `GET`  | `/transfers/{transferId}`              | Get a transfer (owner-scoped)                                      |

`POST /transfers` requires an `Idempotency-Key` header — a client-generated UUID
(uuidv7 recommended). Retrying with the same key replays the original transfer;
reusing it with a different body returns `409`. The transfer is created
`PENDING` and settled asynchronously via Temporal, so poll
`GET /transfers/{id}` for the final `SUCCEEDED`/`FAILED` status.

`transferType` selects the flow. `INTERNAL` (default) moves funds to another
in-ledger account and requires `toAccountRef` (an `ACC-` account ref).
`EXTERNAL` sends funds out through Bank gRPC: `toAccountRef` is an optional
free-text beneficiary id and the `provider` is set from server config — clients
never send it. A `message` is required on every transfer.

`amount`/`currency` are the **transaction intent**. Settlement amounts are
computed server-side from FX rates once the Temporal workflow prepares/moves
money (`sourceAmount`/`destinationAmount`/rates/fee snapshot).

```bash
# Internal P2P transfer
curl -X POST http://localhost:4000/api/v1/transfers \
  -H "Authorization: Bearer <token>" \
  -H 'Idempotency-Key: 0190bf3e-...' \
  -H 'Content-Type: application/json' \
  -d '{"fromAccountRef":"ACC-...","toAccountRef":"ACC-...","amount":"100","currency":"USD","transferType":"INTERNAL","message":"rent"}'

# External transfer (no in-ledger destination)
curl -X POST http://localhost:4000/api/v1/transfers \
  -H "Authorization: Bearer <token>" \
  -H 'Idempotency-Key: 0190bf3f-...' \
  -H 'Content-Type: application/json' \
  -d '{"fromAccountRef":"ACC-...","amount":"100","currency":"USD","transferType":"EXTERNAL","message":"payout"}'
```

Authorization is P2P: the `fromAccountRef` must belong to the caller (otherwise
`403`); the destination may be anyone's. Reads are ownership-scoped by account.
The full request/response schema is in generated `openapi.yaml` (`make openapi`).

## Internal Transfer Flow

```mermaid
sequenceDiagram
    actor C as Client
    participant GW as Traefik
    participant API as Transfer API
    participant DB as PostgreSQL
    participant IRIS as iris CDC
    participant K as Kafka
    participant BR as consumer bridge
    participant T as Temporal
    participant W as transfer-worker
    participant WG as wallet-grpc
    participant FX as fx gRPC

    C->>GW: POST /transfers (Bearer, Idempotency-Key)
    GW->>API: forward + X-User-Id
    API->>DB: BEGIN — INSERT transfer(PENDING)<br/>+ outbox(transfer.requested) — COMMIT
    API-->>C: 202 { transferId, status: PENDING }

    IRIS->>DB: logical replication
    IRIS->>K: publish transfer.requested
    BR->>K: consume transfer.requested
    Note over BR: inbox dedup
    BR->>T: StartWorkflow(transfer-{id})
    T->>W: TransferWorkflow INTERNAL
    W->>DB: LoadTransfer / PrepareInternalMove<br/>(Quote FX + freeze settlement)
    W->>FX: Quote + QuoteFee
    W->>WG: Move (debit+credit+ledger+fee, op-guard)
    WG->>DB: money tx
    W->>DB: MarkTerminal SUCCEEDED + outbox(completed)
    Note over W,DB: business failure → MarkTerminal FAILED<br/>(no money / non-retryable activity error)

    C->>API: GET /transfers/{id}
    API-->>C: SUCCEEDED / FAILED
```

## External Transfer Flow

External transfers hold funds first, then settle after Bank responds. UNKNOWN /
timeout keeps the hold and polls `Bank.Query` — no auto-release.

```mermaid
sequenceDiagram
    actor C as Client
    participant API as Transfer API
    participant DB as PostgreSQL
    participant BR as consumer bridge
    participant T as Temporal
    participant W as transfer-worker
    participant WG as wallet-grpc
    participant BG as bank-grpc

    C->>API: POST /transfers (EXTERNAL)
    API->>DB: PENDING + outbox(transfer.requested)
    API-->>C: 202 PENDING

    BR->>T: StartWorkflow(transfer-{id})
    T->>W: TransferWorkflow EXTERNAL
    W->>DB: PrepareExternalHold<br/>(currency == source, snapshot)
    W->>WG: Hold (available→hold, op-guard)
    WG->>DB: hold tx
    W->>BG: Submit(transferId, amount, currency)

    alt SUCCESS
        BG-->>W: SUCCESS + referenceId
        W->>WG: SettleHold
        W->>DB: MarkTerminal SUCCEEDED + outbox(completed)
    else FAILURE
        BG-->>W: FAILURE + reason
        W->>WG: ReleaseHold
        W->>DB: MarkTerminal FAILED + outbox(failed)
    else UNKNOWN / timeout
        BG-->>W: UNKNOWN
        loop poll until known (hold retained)
            W->>BG: Query(transferId)
        end
        Note over W: never auto-release on UNKNOWN<br/>alert after ~15m; recon manual
    end

    C->>API: GET /transfers/{id}
    API-->>C: SUCCEEDED / FAILED / still in-flight
```

## Multi-Currency & FX Settlement

A transfer separates the **transaction intent** (client request) from the
**settlement** (per-account postings in each account's currency). The Temporal
prepare activity quotes via the `fx` service and freezes the settlement snapshot
before money moves.

Rates come from static config (`fx.rates`, keyed `FROM_TO`). Same-currency
corridors quote at rate `1`. Missing corridors fail with `FX_RATE_UNAVAILABLE`
(non-retryable business failure).

A flat **FX conversion fee** (`fx.fees`, keyed by source currency) is charged
when an internal transfer converts out of the source currency (third `FEE`
ledger entry). Missing/non-positive fee = no fee.

```yaml
fx:
  rates:
    VND_USD: '0.00003924'
    USD_VND: '25484.20'
    USD_EUR: '0.92'
    EUR_USD: '1.0870'
  fees:
    USD: '1'
    VND: '10000'
```

### Internal transfers (cross-currency capable)

| Case                 | Example                             | Ledger entries (on success)                            |
| -------------------- | ----------------------------------- | ------------------------------------------------------ |
| Same currency        | 100 USD, both accounts USD          | `DEBIT 100 USD` + `CREDIT 100 USD`                     |
| Destination converts | 100 USD → VND @ `25484.20`          | `DEBIT 100 USD` + `CREDIT 2548420 VND`                 |
| Source converts      | 10 USD into USD acct from VND + fee | `DEBIT 254842 VND` + `FEE 10000 VND` + `CREDIT 10 USD` |

Cross-currency postings are balanced **per currency**, not by absolute value.

### External transfers (single-currency only)

External transfers do **not** convert. Prepare requires source account currency
== transaction currency (`source_fx_rate = 1`). Mismatch →
`FX_RATE_UNAVAILABLE` before any hold.

| Step / outcome | Ledger entry                          |
| -------------- | ------------------------------------- |
| Hold           | `HOLD`                                |
| Settle success | `DEBIT` (drops hold)                  |
| Settle failure | `RELEASE` (returns hold to available) |
| UNKNOWN        | hold retained                         |

## Worker / Temporal Flow

```mermaid
flowchart TD
    RPREQ[("Kafka topic\ntransfer.requested")]
    RPDONE[("Kafka topic\ntransfer.completed")]
    RPFAIL[("Kafka topic\ntransfer.failed")]
    TDLQ[("transx.transfer.dlq")]
    TRETRY[("transx.transfer.retry-*")]

    subgraph Bridge["consumer — group: wallet-processor"]
      DEDUP{"inbox processed?"}
      START["StartWorkflow\ntransfer-{id}"]
      ESCALATE{"start error?"}
    end

    subgraph Temporal["Temporal + transfer-worker"]
      LOAD["LoadTransfer"]
      TYPE{"type?"}
      INT["INTERNAL\nPrepare → Move → MarkTerminal"]
      EXT["EXTERNAL\nPrepare → Hold → Bank\n→ Settle|Release|poll Query"]
    end

    RPREQ --> DEDUP
    DEDUP -->|yes| SKIP["commit offset"]
    DEDUP -->|no| START
    START --> LOAD
    LOAD --> TYPE
    TYPE -->|INTERNAL| INT
    TYPE -->|EXTERNAL| EXT
    INT -->|MarkTerminal success outbox| RPDONE
    INT -->|MarkTerminal failure outbox| RPFAIL
    EXT -->|MarkTerminal success outbox| RPDONE
    EXT -->|MarkTerminal failure outbox| RPFAIL
    RPDONE --> NOTIF["notification"]
    RPFAIL --> NOTIF
    START --> ESCALATE
    ESCALATE -->|transient| TRETRY
    ESCALATE -->|poison / exhausted| TDLQ
    TRETRY -->|delay elapsed| RPREQ
```

- **iris** publishes every outbox `event_type` as a **separate Kafka topic** of the same name (`transfer.requested` vs `transfer.completed` / `transfer.failed`).
- **consumer** is only the bridge: inbox dedup, Temporal start, delayed-retry
  tiers for transient Temporal/start failures (`transx.transfer.retry-*`), DLQ
  for poison/exhausted starts.
- **transfer-worker** owns money movement and terminal status via activities.
  Wallet operations are idempotent on `(transfer_id, operation)`. MarkTerminal
  is status+outbox only and retries until converge after money has moved.
- **notification** still uses its own inbox + retry/DLQ topics
  (`transx.notification.*`).

## Idempotency

| Layer            | Mechanism                                                            | Location                     |
| ---------------- | -------------------------------------------------------------------- | ---------------------------- |
| **API**          | Unique `(user_id, idempotency_key)` + `request_hash`                 | transfer application service |
| **Kafka bridge** | `inbox_events` `(wallet-processor, transferId)` before StartWorkflow | `cmd/consumer`               |
| **Temporal**     | WorkflowID `transfer-{id}` + `AlreadyStarted` = success              | consumer + Temporal          |
| **Wallet money** | `wallet_operation_guards (transfer_id, operation)`                   | `PostgresMoneyRepository`    |
| **Status**       | MarkTerminal no-op when already terminal                             | transfer repository          |

```mermaid
flowchart LR
    subgraph API["API Layer"]
        A1["POST /transfers"]
        A2{"idempotency key\nexists?"}
        A3["replay or 409"]
        A4["PENDING + outbox"]
        A1 --> A2
        A2 -->|yes| A3
        A2 -->|no| A4
    end

    subgraph Bridge["Kafka Bridge"]
        K1["transfer.requested"]
        K2{"inbox processed?"}
        K3["skip"]
        K4["StartWorkflow"]
        K1 --> K2
        K2 -->|yes| K3
        K2 -->|no| K4
    end

    subgraph Worker["Temporal activities"]
        W1["Wallet op-guard"]
        W2["MarkTerminal status-guard"]
        K4 --> W1 --> W2
    end
```

## Backend Architecture

```
internal/
├── modules/
│   ├── auth/         JSON AT+RT (login/refresh/logout/session), ForwardAuth /check
│   ├── wallet/       accounts, ledger, money repository (gRPC ops)
│   ├── transfer/     transfers, outbox, inbox, transfer application service
│   ├── fx/           rates + fees
│   └── notification/ terminal event notifications
├── platform/         config, postgres, kafka, httpserver, grpc, logger, middleware
├── common/           apperror, kafkatopic, provider (Bank fake)
└── shared/           lifecycle, pgconv
cmd/
├── api/              HTTP handlers + routes
├── consumer/         Kafka→Temporal bridge
├── worker/           TransferWorkflow + activities
├── grpc/             fx / wallet / bank handlers
└── notification/     terminal consumers
cli/                  service runners
```

Conventions:

- **IDs are UUID v7** — DB defaults to `uuidv7()` (Postgres 18).
- **Money is `decimal.Decimal`** → `NUMERIC(20,4)`; FX rates `NUMERIC(20,12)`.
- **Transaction intent vs settlement** — clients never send rates/settlement amounts.
- **Errors** return `*apperror.AppError`; `DomainErrorHandler` maps HTTP status.
- **Config** env override: `SECTION__KEY` (e.g. `TEMPORAL__HOST_PORT`,
  `BANK__MODE`, `WALLET__GRPC_ADDRESS`).
- **Money never settles across a network call in one tx** — EXTERNAL holds first,
  then settles/releases after Bank; UNKNOWN keeps the hold.

## Key Docs

- Product requirements: `docs/prd.md`
- System architecture: `docs/system-architecture.md`
- Hybrid auth / Auth BFF: `docs/ssr.md`
- Frontend (RR + Auth BFF): `frontend/README.md`
- OpenAPI spec: `backend/openapi.yaml`
- Temporal saga plan: `plans/260711-2300-temporal-saga-transfer-orchestration/`
- RR framework + hybrid auth plan: `plans/260715-1333-rrv7-framework-hybrid-auth/`
