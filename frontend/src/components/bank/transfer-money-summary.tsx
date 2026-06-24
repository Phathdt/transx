import type { DtoTransferResponse } from '#/lib/api/generated/models'
import type { TransferDirection } from '#/lib/transfer/transfer-direction'

function Row({ label, value }: { label: string; value?: string }) {
  if (!value) return null
  return (
    <div className="flex items-center justify-between py-1.5 text-sm">
      <span className="text-muted-foreground">{label}</span>
      {/* Money values stay strings exactly as the API returns them. */}
      <span className="font-medium tabular-nums">{value}</span>
    </div>
  )
}

/**
 * Renders the monetary breakdown of a transfer. All amounts are rendered as the
 * raw decimal strings from the API; no float arithmetic. The direction (relative
 * to the caller) swaps the counterparty rows: a received transfer shows the
 * sending account, a sent/own transfer shows the receiver.
 */
export function TransferMoneySummary({
  transfer,
  direction = 'sent',
}: {
  transfer: DtoTransferResponse
  direction?: TransferDirection
}) {
  const txn =
    transfer.transactionAmount && transfer.transactionCurrency
      ? `${transfer.transactionAmount} ${transfer.transactionCurrency}`
      : transfer.transactionAmount
  const dest =
    transfer.destinationAmount && transfer.destinationCurrency
      ? `${transfer.destinationAmount} ${transfer.destinationCurrency}`
      : transfer.destinationAmount
  const fee =
    transfer.feeAmount && transfer.feeCurrency
      ? `${transfer.feeAmount} ${transfer.feeCurrency}`
      : transfer.feeAmount

  const received = direction === 'received'

  return (
    <div className="divide-y">
      <Row label="Transaction" value={txn} />
      {received ? (
        <Row label="From" value={transfer.fromAccountRef} />
      ) : (
        <>
          <Row label="Receiver" value={transfer.toAccountName} />
          <Row label="Account" value={transfer.toAccountRef} />
        </>
      )}
      <Row label="Destination" value={dest} />
      <Row label="Fee" value={fee} />
      <Row label="Message" value={transfer.message} />
    </div>
  )
}
