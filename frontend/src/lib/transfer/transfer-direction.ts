import type { DtoTransferResponse } from '#/lib/api/generated/models'

export type TransferDirection = 'sent' | 'received' | 'self'

/**
 * Classifies a transfer relative to the caller's own accounts. A transfer where
 * one of my accounts is the source is "sent"; where one of my accounts is the
 * destination it is "received". When both ends are mine (an internal move
 * between my own wallets) it is "self". Falls back to "sent" when the owned-set
 * is unknown, matching the historical single-direction view.
 */
export function transferDirection(
  transfer: Pick<DtoTransferResponse, 'fromAccountRef' | 'toAccountRef'>,
  ownedRefs: ReadonlySet<string>,
): TransferDirection {
  const fromMine = Boolean(
    transfer.fromAccountRef && ownedRefs.has(transfer.fromAccountRef),
  )
  const toMine = Boolean(
    transfer.toAccountRef && ownedRefs.has(transfer.toAccountRef),
  )
  if (fromMine && toMine) return 'self'
  if (toMine) return 'received'
  return 'sent'
}

/**
 * The counterparty account ref to display for a transfer: the other end from
 * the caller's perspective. For a received transfer that is the source; for a
 * sent transfer it is the destination.
 */
export function transferCounterparty(
  transfer: Pick<DtoTransferResponse, 'fromAccountRef' | 'toAccountRef'>,
  direction: TransferDirection,
): string | undefined {
  return direction === 'received'
    ? transfer.fromAccountRef
    : transfer.toAccountRef
}
