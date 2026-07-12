# Migrating `outbox-replayer` to iris (CDC)

Status: viable — blocking questions resolved by iris `sink.outbox` mode (2026-07-12)
Scope: replace the in-house `outbox-replayer` service with [phathdt/iris](https://github.com/phathdt/iris), a Postgres logical-replication → Kafka CDC engine.

iris now ships an **outbox mode** that derives the Kafka *key*, *topic*, and
*body* from row columns. This closes the three mismatches that previously made
migration unsafe. The transfer contract can be preserved **without splitting the
outbox table and without rewriting any consumer.**

---

## 1. What we have today

The transfer flow uses the classic **transactional outbox** pattern.

- The wallet module writes an `outbox_events` row in the **same transaction** as
  the state change it describes (`postgres_transfer_repository.go` — `Create`,
  `ExecuteInternalTransfer`, `failTx` all call `insertTransferOutbox`).
- The `outbox-replayer` service (`cmd/replayer/publisher.go`) polls
  `outbox_events` every 500ms, batches 100 pending rows, and per row:
  1. Routes to a topic by the `event_type` column (`mapEventTypeToTopic`).
  2. Sets the Kafka message key = `aggregate_id` (FIFO per transfer).
  3. Publishes the raw `payload` jsonb as the body.
  4. Marks the row `PUBLISHED` (guarded by `status='PENDING'`).
- Single-instance by design (no row lock; ordering relies on one replayer).

### The contract downstream consumers depend on

| Property | Current value | Source column |
|---|---|---|
| Topics | `transfer.requested`, `transfer.provider.requested`, `transfer.completed`, `transfer.failed` | `event_type` |
| Message key | `aggregate_id` (transfer UUID) | `aggregate_id` |
| Message body | `{"transferId": "<uuid>"}` | `payload` (jsonb) |
| Consumers | `cmd/consumer` (processor, provider, retries), `cmd/notification` — each subscribes to a **specific** topic and parses `msg.Value` as `TransferEventPayload`; retries republish keyed by `msg.Key` | — |

Key fact: **`event_type` values equal the topic names** (`kafkatopic.go` ==
`entities.EventTransfer*`). So iris identity routing on `event_type` produces the
exact topics consumers already subscribe to.

---

## 2. How iris outbox mode maps to our contract

iris streams `outbox_events` via logical replication and, in `sink.outbox` mode,
reads three columns from `event.After` of each INSERT:

| iris config | reads column | replaces replayer logic | result |
|---|---|---|---|
| `route_field: event_type` | `event_type` | `mapEventTypeToTopic` | topic = column value (identity) |
| `key_field: aggregate_id` | `aggregate_id` | `key := aggregate_id` | per-aggregate ordering preserved |
| `payload_field: payload` | `payload` | publish raw `payload` | body = `{"transferId": …}`, no envelope |

All three columns already exist. **No schema split (old Option B is obsolete) and
no consumer changes** — the bytes on the wire are byte-for-byte what the replayer
produces today.

---

## 3. What changes vs. what stays

### Stays unchanged
- `outbox_events` table shape and the transactional write in
  `insertTransferOutbox` (event still staged in the business tx — this is the
  whole point of the outbox pattern and iris does not replace it).
- All Kafka topic names.
- Every consumer (`cmd/consumer`, `cmd/notification`), the retry/DLQ machinery,
  and `TransferEventPayload` parsing.

### Changes
- **Delete** `cmd/replayer/`, `cli/outbox_replayer.go`, and the replayer wiring.
- **`status` / `published_at` become vestigial.** iris tracks progress via the
  replication slot LSN, not a status column. Options:
  - Keep the columns, stop writing/reading them (smallest diff), or
  - Drop them in a later migration once nothing references them.
  - Remove `OutboxRepository.ListPending` / `MarkPublished` usage (only the
    replayer calls them; verify with grep before deleting the methods).
- **Postgres** must run logical replication (see §4).
- **docker-compose**: swap the `outbox-replayer` service for an `iris` service.

---

## 4. Postgres prerequisites

- `wal_level=logical` — add to the postgres service command in
  `docker-compose.yml`: `command: ["postgres", "-c", "wal_level=logical"]`.
- A role with `REPLICATION`, a replication slot, and a publication for
  `outbox_events` (iris creates/uses these per its config — confirm from its
  docs whether it auto-creates the slot/publication or expects them present).
- **Operational note:** a stuck/lagging slot retains WAL and can fill disk.
  Add slot-lag monitoring before production. This is the main new operational
  burden the migration introduces.

`outbox_events` INSERTs populate `event.After` fully regardless of
`REPLICA IDENTITY`, so no replica-identity change is needed for insert-only
streaming.

---

## 5. Proposed iris config

```yaml
# iris.yaml
source:
  type: postgres
  dsn: postgres://transx:transx@postgres:5432/transx?sslmode=disable
  slot: iris_outbox
  tables:
    - outbox_events

sink:
  type: kafka
  brokers: redpanda:29092
  outbox:
    route_field: event_type      # topic = event_type value (transfer.requested, …)
    key_field: aggregate_id      # per-transfer ordering
    payload_field: payload       # raw {"transferId": …}, no CDC envelope

# retry / DLQ / observability per iris defaults — set to match current behavior
```

> Verify the exact key names (`slot`, `tables`, `route_field`, etc.) against the
> current iris README before wiring — this reflects the documented outbox keys
> as of 2026-07-12.

docker-compose service (replaces `outbox-replayer`):

```yaml
  iris:
    image: phathdt/iris   # or build from source
    container_name: transx-iris
    command: ["--config", "/app/iris.yaml"]
    volumes:
      - ./iris.yaml:/app/iris.yaml:ro
    depends_on:
      postgres:
        condition: service_healthy
      migrate:
        condition: service_completed_successfully
      redpanda:
        condition: service_healthy
      redpanda-init:
        condition: service_completed_successfully
    restart: unless-stopped
```

Keep iris **single-instance** (one slot) — same constraint as the replayer today,
so no availability change either way.

---

## 6. Work breakdown

Infra:
- [ ] Enable `wal_level=logical` on postgres in `docker-compose.yml`.
- [ ] Add `iris.yaml`; confirm iris auto-creates slot/publication or add a
      bootstrap step (mirror the `redpanda-init` pattern if needed).
- [ ] Replace `outbox-replayer` service with the `iris` service.

Backend (`backend/`):
- [ ] Delete `cmd/replayer/`, `cli/outbox_replayer.go`, and remove its cli
      subcommand registration in `main.go`.
- [ ] Grep for `ListPending` / `MarkPublished` / `published_at` / `status` on
      `outbox_events`; remove the now-dead repository methods and sqlc queries,
      run `make sqlc` + `make mock`.
- [ ] Decide: keep `status`/`published_at` columns (no-op) or drop later.
- [ ] Update `README.md` / `CLAUDE.md` command list (no more `outbox-replayer`).

Validation:
- [ ] `make check` (sqlc + fmt + vet + lint + test + coverage ≥ 90%).
- [ ] `make test-integration` — create transfer → assert event lands on the
      correct topic, key == transfer UUID, body == `{"transferId": …}`, consumer
      processes, ledger balances, notification row written.
- [ ] Ordering test: many concurrent transfers → assert per-transfer event order
      preserved on each topic (validates `key_field`).
- [ ] Restart/crash test: kill iris mid-stream → assert no lost/duplicated events
      (slot resumes at last LSN; consumers are already idempotent via inbox
      dedup + PENDING-guarded state transitions).

---

## 7. Trade-off summary (updated)

| | outbox-replayer (today) | iris outbox mode |
|---|---|---|
| Topic routing by `event_type` | native | `route_field` ✅ |
| Ordering per transfer | key=aggregate_id | `key_field` ✅ |
| Payload shape | raw `{transferId}` | `payload_field` → raw ✅ |
| Consumer changes | — | **none** ✅ |
| Outbox schema changes | — | none (status/published_at go vestigial) ✅ |
| Transactional outbox write | kept | kept (iris only reads) ✅ |
| Infra deps | PG + Kafka | + logical replication, slot, WAL monitoring ⚠️ |
| Single-instance constraint | yes | yes (one slot) — no change |
| Code to maintain | ~110-line service + tests | external binary + `iris.yaml` |
| Observability | app logs | Prometheus + OTel built-in ✅ |
| Poll latency | up to 500ms | streaming (lower latency) ✅ |

### Recommendation

Migration is now **technically clean**: iris outbox mode preserves the exact wire
contract, deletes a service we maintain, lowers publish latency, and adds
built-in metrics/tracing — at the cost of one new operational concern (logical
replication slot + WAL retention monitoring).

Reasonable to proceed **if** you are comfortable running logical replication in
production and adding slot-lag monitoring. If that operational cost is unwanted,
the current replayer remains correct and simpler to operate.

Suggested rollout: dev/compose first → run iris and replayer in parallel briefly
against separate topics to diff output → cut consumers over → delete replayer.

---

## Unresolved questions

1. Does iris **auto-create** the replication slot + publication, or must a
   bootstrap step create them? (drives whether we need an `iris-init` service)
2. Exact iris config key names for slot/tables/retry/DLQ — confirm against the
   current README before wiring.
3. Do we keep `status`/`published_at` columns as vestigial, or drop them in a
   follow-up migration?
4. Production Postgres: is enabling `wal_level=logical` + slot/WAL monitoring
   operationally acceptable?
