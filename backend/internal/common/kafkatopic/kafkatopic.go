// Package kafkatopic is the single source of truth for Kafka topic names and
// the event-type strings carried on those topics. Keeping them here avoids
// scattered string literals across producers, consumers and the watcher, and
// documents which events flow over which topic.
package kafkatopic

import "time"

// Topic names. Each service owns its own DLQ failure stream.
const (
	// Orders carries raw chain facts emitted by the indexer/watcher
	// (deposit.detected, settlement.completed, order.refunded). The order
	// service consumes it.
	Orders = "transx.orders"

	// OrderFacts carries facts the order service emits after persisting state
	// (order.deposited); the settlement service consumes it.
	OrderFacts = "transx.order-facts"

	// SettlementFacts carries facts the settlement service emits after a chain
	// action (settle.broadcast); the order service consumes it.
	SettlementFacts = "transx.settlement-facts"

	// OrderDLQ is the order service's dead-letter queue.
	OrderDLQ = "transx.order.dlq"

	// SettlementDLQ is the settlement service's dead-letter queue.
	SettlementDLQ = "transx.settlement.dlq"

	// Transfer lifecycle topics. TransferRequested is consumed by the
	// Kafka→Temporal bridge; Completed/Failed feed the notification service.
	TransferRequested = "transfer.requested"
	TransferCompleted = "transfer.completed"
	TransferFailed    = "transfer.failed"

	// TransferDLQ is the transfer bridge's dead-letter queue (poison /
	// exhausted start-workflow retries).
	TransferDLQ = "transx.transfer.dlq"

	// NotificationDLQ is the notification service's dead-letter queue.
	NotificationDLQ = "transx.notification.dlq"
)

// Delayed-retry topics. A handler that fails on the main topic escalates the
// message through these tiered retry topics (short → long delay) before it is
// finally parked in the DLQ. Each tier has a dedicated consumer that holds the
// message until its delay elapses, then republishes it onto the main topic.
//
// Escalation is driven by an attempt counter carried in the message header
// (HeaderRetryAttempt): attempt 0 → first retry tier, attempt 1 → second tier,
// and so on. Once the counter exceeds the last tier, the message goes to the
// service DLQ. The retry topic itself only encodes the delay, not the attempt.
const (
	OrderRetry6s  = "transx.order.retry-6s"
	OrderRetry30s = "transx.order.retry-30s"
	OrderRetry5m  = "transx.order.retry-5m"

	SettlementRetry6s  = "transx.settlement.retry-6s"
	SettlementRetry30s = "transx.settlement.retry-30s"
	SettlementRetry5m  = "transx.settlement.retry-5m"

	TransferRetry6s  = "transx.transfer.retry-6s"
	TransferRetry30s = "transx.transfer.retry-30s"
	TransferRetry5m  = "transx.transfer.retry-5m"

	NotificationRetry6s  = "transx.notification.retry-6s"
	NotificationRetry30s = "transx.notification.retry-30s"
	NotificationRetry5m  = "transx.notification.retry-5m"
)

// Message headers used by the delayed-retry machinery.
const (
	// HeaderRetryAttempt holds the decimal retry attempt count (0-based). The
	// main handler reads it to pick the next retry tier; the retry consumer
	// increments it when republishing onto the main topic.
	HeaderRetryAttempt = "x-retry-attempt"

	// HeaderRetryAt holds the unix-millis timestamp before which the retry
	// consumer must not republish the message. Set when a message is parked on
	// a retry topic (now + tier delay).
	HeaderRetryAt = "x-retry-at"

	// HeaderRetryFrom holds the main topic a parked message must be replayed to
	// once its delay elapses.
	HeaderRetryFrom = "x-retry-from"

	// HeaderError holds the most recent failure message, carried through the
	// retry tiers and into the DLQ for observability.
	HeaderError = "x-error"
)

// RetryStage describes one tier of the delayed-retry escalation: the topic that
// parks the message and how long it is held before being replayed.
type RetryStage struct {
	Topic string
	Delay time.Duration
}

// orderRetryStages and settlementRetryStages are the ordered escalation tiers
// per service. Index == retry attempt: stage[0] is used on the first failure,
// stage[1] on the second, and so on. After the last stage the message is DLQ'd.
var (
	orderRetryStages = []RetryStage{
		{Topic: OrderRetry6s, Delay: 6 * time.Second},
		{Topic: OrderRetry30s, Delay: 30 * time.Second},
		{Topic: OrderRetry5m, Delay: 5 * time.Minute},
	}

	settlementRetryStages = []RetryStage{
		{Topic: SettlementRetry6s, Delay: 6 * time.Second},
		{Topic: SettlementRetry30s, Delay: 30 * time.Second},
		{Topic: SettlementRetry5m, Delay: 5 * time.Minute},
	}

	transferRetryStages = []RetryStage{
		{Topic: TransferRetry6s, Delay: 6 * time.Second},
		{Topic: TransferRetry30s, Delay: 30 * time.Second},
		{Topic: TransferRetry5m, Delay: 5 * time.Minute},
	}

	notificationRetryStages = []RetryStage{
		{Topic: NotificationRetry6s, Delay: 6 * time.Second},
		{Topic: NotificationRetry30s, Delay: 30 * time.Second},
		{Topic: NotificationRetry5m, Delay: 5 * time.Minute},
	}
)

// OrderRetryStages returns the order service's retry escalation tiers.
func OrderRetryStages() []RetryStage { return orderRetryStages }

// SettlementRetryStages returns the settlement service's retry escalation tiers.
func SettlementRetryStages() []RetryStage { return settlementRetryStages }

// TransferRetryStages returns the transfer bridge's retry escalation tiers.
func TransferRetryStages() []RetryStage { return transferRetryStages }

// NotificationRetryStages returns the notification service's retry escalation tiers.
func NotificationRetryStages() []RetryStage { return notificationRetryStages }

// NextRetryStage returns the retry tier for the given 0-based attempt count and
// ok=true, or ok=false when the attempts are exhausted and the caller should
// route to the DLQ instead.
func NextRetryStage(stages []RetryStage, attempt int) (RetryStage, bool) {
	if attempt < 0 || attempt >= len(stages) {
		return RetryStage{}, false
	}
	return stages[attempt], true
}

// Event-type strings carried in message payloads. These are the "event" field
// values on the fact payloads and order_events Action values.
const (
	// Chain facts on the Orders topic (produced by the watcher).
	EventDepositDetected     = "deposit.detected"
	EventSettlementCompleted = "settlement.completed"
	EventOrderRefunded       = "order.refunded"

	// Fact on the OrderFacts topic (order → settlement).
	EventOrderDeposited = "order.deposited"

	// Fact on the SettlementFacts topic (settlement → order).
	EventSettleBroadcast = "settle.broadcast"

	// Order lifecycle events recorded in order_events (no dedicated topic).
	EventOrderAccepted         = "order.accepted"
	EventSettlementRequested   = "settlement.requested"
	EventSettlementBroadcasted = "settlement.broadcasted"
	EventOrderExpired          = "order.expired"
)
