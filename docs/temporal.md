# Product Requirements Document (PRD)

## TransX – Next Phase Roadmap

### Version

v0.2

### Status

Draft

---

# 1. Overview

The current version of **TransX** provides a reliable transfer orchestration platform powered by **Temporal**, supporting both **internal** and **external** transfers with Saga-based workflows, retries, compensation, and event-driven integrations.

The next phase aims to transform TransX from a transfer engine into a complete payment workflow platform by introducing long-running business processes, scheduling capabilities, operational tooling, and risk management.

---

# 2. Goals

### Primary Goals

- Expand Temporal usage for business workflows.
- Improve operational visibility.
- Support future banking/payment use cases.
- Keep side effects event-driven through Kafka.
- Maintain clear separation between orchestration and business services.

---

# 3. Proposed Features

---

## Feature 1 — Scheduled Transfer

**Status: Implemented** (v1 scope: create with `executeAt`, cancel-only, no
edit/reschedule — see `plans/260716-1504-scheduled-transfer/plan.md` and
`docs/system-architecture.md#scheduled-transfers`).

### Problem

Users should be able to schedule transfers for a future date and time.

### Example

> Transfer $500 to Alice on August 8th at 09:00.

### Requirements

- User specifies execution time (`executeAt`, optional, RFC3339, now < t <= now+90d).
- v1 is **cancel-only** while `SCHEDULED` (no edit/reschedule).
- Automatically execute at scheduled time (no funds held until execute).
- Retry on transient failures.
- Preserve idempotency.

### Temporal Workflow

```
Create Scheduled Transfer
        │
        ▼
Sleep Until Execute Time
        │
        ▼
Execute Existing Transfer Workflow
```

### Priority

⭐⭐⭐⭐⭐

---

## Feature 2 — Recurring Transfer

### Problem

Support automatic recurring payments.

Examples:

- Monthly rent
- Loan repayment
- Subscription payment

### Requirements

- Daily
- Weekly
- Monthly
- Custom interval

Support:

- Pause
- Resume
- Cancel

### Temporal Workflow

```
Loop
   │
Transfer
   │
Sleep Until Next Schedule
   │
Continue-As-New
```

### Priority

⭐⭐⭐⭐⭐

---

## Feature 3 — Manual Approval Workflow

### Problem

Large transfers may require manual approval.

### Requirements

Configurable rules:

- Amount threshold
- Destination account
- Business account

Workflow:

```
Create Transfer
      │
Need Approval?
      │
      ▼
Wait Signal
      │
 ┌────┴────┐
 │         │
Approve  Reject
```

Support:

- Approval timeout
- Auto rejection
- Multiple approvers (future)

### Priority

⭐⭐⭐⭐⭐

---

## Feature 4 — Hold Expiration

### Problem

External transfers may remain in HOLD forever if upstream never responds.

### Requirements

- Configurable expiration
- Automatic release
- Audit log
- Notification

Workflow

```
Hold
   │
Wait
   │
Timeout
   │
Release Hold
```

### Priority

⭐⭐⭐⭐

---

## Feature 5 — Manual Intervention

### Problem

Operations team should be able to intervene.

Actions:

- Retry
- Resume
- Force Success
- Force Failure
- Cancel Workflow

Support Temporal Signals.

### Priority

⭐⭐⭐⭐

---

## Feature 6 — Fraud / AML Review

### Problem

High-risk transfers require compliance review.

Workflow

```
Transfer
    │
Risk Engine
    │
High Risk?
    │
Wait Human Review
```

Possible outcomes:

- Approve
- Reject
- Escalate

### Priority

⭐⭐⭐⭐

---

## Feature 7 — Escalation Workflow

### Problem

Long-running transfers should automatically notify operators.

Example

```
Unknown > 15 min
        │
Alert Ops

Unknown > 1 hour
        │
Page Engineer

Unknown > 6 hours
        │
Escalate
```

### Priority

⭐⭐⭐⭐

---

## Feature 8 — Webhook Delivery

### Problem

Merchants need asynchronous callbacks.

Requirements

- Retry
- Exponential backoff
- Signature
- Dead-letter handling

This should remain event-driven.

```
Transfer Completed
        │
Kafka
        │
Webhook Worker
```

### Priority

⭐⭐⭐⭐

---

## Feature 9 — Notification Service

Current architecture is already event-driven.

```
Transfer
    │
Outbox
    │
CDC
    │
Kafka
    │
Notification
```

Enhancements:

- Push notification
- Email
- SMS
- In-app notification

Notification should **not** be part of the Temporal workflow.

### Priority

⭐⭐⭐

---

## Feature 10 — Transfer Limits

Support configurable rules.

Examples

- Daily transfer limit
- Monthly limit
- Per-user limit
- Per-bank limit

Workflow

```
Validate Limit
      │
Exceeded?
      │
Reject
```

### Priority

⭐⭐⭐

---

## Feature 11 — Operational Dashboard

Expose metrics

- Running workflows
- Failed workflows
- Retry count
- Compensation count
- Average completion time
- External bank latency

Priority

⭐⭐⭐⭐

---

## Feature 12 — Audit Timeline

Every transfer should expose a timeline.

Example

```
12:00 Created

12:00 Hold

12:01 Submitted

12:03 Unknown

12:10 Retry

12:20 Success

12:20 Notification Published
```

Useful for:

- Customer support
- Debugging
- Compliance

Priority

⭐⭐⭐⭐

---

# 4. Future Enhancements

- Multi-bank routing
- FX conversion workflow
- Batch transfers
- Payroll processing
- Cross-border settlement
- Chargeback workflow
- Refund workflow
- Merchant payout
- Scheduled settlement
- Human task integration
- Payment orchestration across providers

---

# 5. Architecture Principles

## Temporal Responsibilities

Temporal should orchestrate **business workflows** only.

Examples:

- Transfer
- Settlement
- Scheduled execution
- Recurring payment
- Approval
- Fraud review
- Compensation
- Escalation

---

## Kafka Responsibilities

Kafka should remain responsible for **event distribution**.

Examples:

- Notifications
- Webhooks
- Analytics
- Audit
- Search indexing
- Cache invalidation

---

## Outbox Pattern

All business state changes must publish domain events via the Outbox pattern.

Examples:

- TransferCreated
- TransferSubmitted
- TransferSucceeded
- TransferFailed
- TransferCancelled
- HoldCreated
- HoldReleased

---

# 6. Long-Term Vision

TransX should evolve from a transfer processing service into a **workflow-driven payment orchestration platform**.

Core principles:

- Workflow orchestration powered by Temporal
- Event-driven integrations powered by Kafka
- Reliable state transitions through Saga patterns
- Extensible architecture with domain events
- Clear separation between orchestration, business logic, and side effects
