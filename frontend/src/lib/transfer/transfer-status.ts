/**
 * Transfer status values mirror the backend `transfer.go` enum. Non-terminal
 * states keep polling; terminal states stop and may show recovery guidance.
 */
export const NON_TERMINAL_STATUSES = [
  'PENDING',
  'SCHEDULED',
  'RESERVED',
  'PROCESSING',
  'SUBMITTED',
] as const

export const TERMINAL_STATUSES = [
  'SUCCEEDED',
  'FAILED',
  'CANCELLED',
  'REVERSED',
  'UNKNOWN',
] as const

export function isTerminalStatus(status: string | undefined): boolean {
  if (!status) return false
  return (TERMINAL_STATUSES as readonly string[]).includes(status)
}

/** Known failure reasons from the backend; unknown values fall back to raw. */
const FAILURE_REASON_COPY: Record<string, string> = {
  INSUFFICIENT_FUNDS: 'Insufficient funds',
  ACCOUNT_NOT_ACTIVE: 'Source account is not active',
  DEST_NOT_ACTIVE: 'Destination account is not active',
  PROVIDER_REJECTED: 'Payment provider rejected the transfer',
  FX_RATE_UNAVAILABLE: 'Exchange rate unavailable',
}

export function formatFailureReason(reason: string | undefined): string | null {
  if (!reason) return null
  return FAILURE_REASON_COPY[reason] ?? reason
}
