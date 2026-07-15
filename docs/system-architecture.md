# System Architecture

## Services

Each backend service is a subcommand of the single `transx` binary
(`backend/main.go`, urfave/cli). See `CLAUDE.md` for the run commands and the
process-level responsibilities of each subcommand (`auth`, `wallet`,
`transfer`, `consumer`, `notification`, `fx`, `wallet-grpc`, `bank-grpc`,
`transfer-worker`).

## Transfer orchestration (Temporal)

Transfer lifecycle is orchestrated by **Temporal**, not by the old Kafka
provider/retry processors:

```text
HTTP transfer API
  → transfers(PENDING) + outbox(transfer.requested)
iris CDC
  → Kafka topic transfer.requested
consumer (bridge only)
  → Temporal StartWorkflow(WorkflowID=transfer-{id})
transfer-worker
  → activities: FX Quote, Wallet gRPC (Move/Hold/Settle/Release),
                Bank gRPC (Submit/Query), MarkTerminal (status+outbox)
iris
  → transfer.completed / transfer.failed → notification
```

- **INTERNAL:** prepare (quote + settlement snapshot) → `Wallet.Move` (1 tx) →
  `MarkTerminal(SUCCEEDED|FAILED)`.
- **EXTERNAL:** currency check → `Wallet.Hold` → `Bank.Submit` → SUCCESS
  `SettleHold+MarkTerminal` / FAILURE `ReleaseHold+MarkTerminal` / UNKNOWN
  keep hold + poll `Bank.Query` (no auto-release).
- Money (Wallet) and status (Transfer) are separate transactions; activities
  are idempotent via wallet operation guards and transfer status guards.
  `MarkTerminal` uses a long Temporal retry budget so status converges after
  money movement.

`consumer` is only the Kafka→Temporal bridge (inbox dedup + `ExecuteWorkflow` +
start-failure retry tiers). It does not move money. The HTTP `stub-provider`
and the provider/retry Kafka consumers are removed; Bank gRPC replaces them.

## gRPC services

Internal gRPC services under `cmd/grpc/` with protos in `proto/`:

- **Wallet** (`wallet-grpc`, `proto/wallet/v1/wallet.proto`) exposes
  `Move`/`Hold`/`SettleHold`/`ReleaseHold`. Every RPC carries `transfer_id` +
  `operation`; `PostgresMoneyRepository` checks a `wallet_operation_guards` row
  for that pair before applying the movement and writes it after, in the same
  transaction as the movement, so a repeated call with the same pair is a
  no-op that returns the current balance. Money crosses the wire as decimal
  strings. Called from Temporal activities in `transfer-worker`.
- **Bank** (`bank-grpc`, `proto/bank/v1/bank.proto`) exposes `Submit`/`Query`.
  It is stateless and mode-driven (`always_success` / `always_failure` /
  `always_timeout`, from `bank.mode`): both RPCs derive their outcome from the
  shared `provider.FakeProviderClient`, so `Query` recomputes the same result
  `Submit` would return. Called from Temporal EXTERNAL saga activities.
- **FX** (`fx`) exposes `Quote`/`QuoteFee`; called from prepare activities.

## Module ownership boundary

The `wallet` and `transfer` HTTP services, and the `consumer` worker, share
one Go module and one Postgres `public` schema. Within that shared codebase,
two DDD packages under `backend/internal/modules/` divide table ownership:

| Package | Owns tables | Notes |
| --- | --- | --- |
| `internal/modules/wallet` | `accounts`, `ledger_entries`, `wallet_operation_guards` | Balances, holds, and the Wallet gRPC idempotency guard. |
| `internal/modules/transfer` | `transfers`, `outbox_events` | Also `inbox_events` (consumer dedup), held here temporarily. |

Each package's `infrastructure/repositories` calls only its own
`infrastructure/gen` (sqlc-generated) queries. `domain/entities` and
`domain/interfaces` for a package describe only the entities/ports that
package owns.

### Cross-module dependency: transfer reads wallet's accounts

`transfer`'s application service (`TransferService`) depends on wallet's
`AccountRepository` interface (`internal/modules/wallet/domain/interfaces`) to:

- authorize that the caller owns the source account,
- validate the destination account exists and is active,
- look up a beneficiary's holder name for beneficiary confirmation.

This is a read-only dependency — `transfer` never writes to `accounts`. It is
a legitimate temporary coupling: a future phase will move this behind a gRPC
boundary once `wallet` and `transfer` become independently deployable
services with separate databases. Until then, `transfer` importing wallet's
domain interfaces (not its repository implementation or `gen` package) is the
accepted shape of the dependency.

### The one sanctioned exception: `ExecuteInternalTransfer` / `ReserveExternalTransfer` / `SettleExternalTransfer`

`PostgresTransferRepository` (in `internal/modules/transfer/infrastructure/repositories`)
holds a second `*gen.Queries` bound to wallet's `infrastructure/gen` package
(`walletQ`), passed in via its constructor. `ExecuteInternalTransfer`,
`ReserveExternalTransfer` and `SettleExternalTransfer` each open one Postgres
transaction (`postgres.WithTx`) and, inside it, rebind both `q` (transfer's
own queries: transfer status, outbox events) and `walletQ` (wallet's queries:
account debit/credit/hold, ledger entries) to the same `pgx.Tx` before using
either. This keeps the money movement and the status/outbox advancement
atomic — they commit or roll back together.

This is the one place `transfer`'s infrastructure layer directly touches
wallet's tables. It is intentional and commented in the code as such. It
exists because splitting the atomic money-movement transaction into two
services (a saga) is a larger change than a package reorganization — that
split is deferred to a later phase when `wallet` and `transfer` move to
separate services/databases and the transaction boundary has to become a
proper distributed saga (e.g. orchestrated via outbox + compensating events)
instead of a single Postgres transaction.

### Enforcement

There is no lint rule (e.g. golangci-lint depguard) enforcing this boundary —
no golangci-lint config is tracked in this repository. The boundary is
enforced by convention and code review: a PR that adds a new import from
`transfer` into wallet's `infrastructure/gen` or `infrastructure/repositories`
(or vice versa) outside of the sanctioned exception above should be flagged
in review.

### Shared value objects

Logic used by both packages (currency code validation/normalization,
page/pageSize clamping, the wallet account external-ref format) lives under
`internal/common/` (`currency`, `pagination`, `accountref`) rather than being
duplicated in both `wallet` and `transfer`, or introducing a dependency of one
package on the other's application layer for unrelated value objects.

The provider HTTP client/contract (`internal/common/provider`) is also
shared: `wallet` uses it read-only for beneficiary lookup
(`ProviderAccountLookupClient`), and `transfer` uses it to submit external
transfers (`ProviderClient`). Keeping the wire contract (`http_contract.go`)
in one place means the two call sites and the stub server cannot drift on
field names.


## Notification service: dispatch audit vs in-app inbox

The `notification` process owns two distinct surfaces that must not be confused:

| Surface | Table | Purpose | Public API |
| --- | --- | --- | --- |
| Dispatch audit | `notifications` | EMAIL/PUSH send attempts (SENT/FAILED) for ops | **None** (internal only) |
| In-app inbox | `user_inbox_items` | User-facing messages (read/unread) | `GET/POST /api/v1/inbox/*` |

On each terminal Kafka event (`transfer.completed` / `transfer.failed`):

1. `CreateInboxItems` inserts one row per recipient (`user_id, type, transfer_id` unique when `transfer_id` present). Recipients: always the sender; also the destination account owner when that account is in-system (typical INTERNAL). EXTERNAL free-text destinations → sender only.
2. `Notify` still logs EMAIL/PUSH dispatch attempts (unchanged audit path).

Inbox creation is independent of channel send success so users still see the event when a notifier fails. Traefik routes `PathPrefix(/api/v1/inbox)` (priority 90, ForwardAuth) to the notification service HTTP port.

Frontend: AppShell bell polls `GET /inbox/unread-count`, lists via `GET /inbox`, opens detail via `GET /inbox/:id` (auto-marks read), and can `POST /inbox/read-all`.
