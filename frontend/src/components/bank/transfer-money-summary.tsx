import type { DtoTransferResponse } from '#/lib/api/generated/models'

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
 * raw decimal strings from the API; no float arithmetic.
 */
export function TransferMoneySummary({
  transfer,
}: {
  transfer: DtoTransferResponse
}) {
  const txn =
    transfer.transactionAmount && transfer.transactionCurrency
      ? `${transfer.transactionAmount} ${transfer.transactionCurrency}`
      : transfer.transactionAmount
  const source =
    transfer.sourceAmount && transfer.sourceCurrency
      ? `${transfer.sourceAmount} ${transfer.sourceCurrency}`
      : transfer.sourceAmount
  const dest =
    transfer.destinationAmount && transfer.destinationCurrency
      ? `${transfer.destinationAmount} ${transfer.destinationCurrency}`
      : transfer.destinationAmount
  const fee =
    transfer.feeAmount && transfer.feeCurrency
      ? `${transfer.feeAmount} ${transfer.feeCurrency}`
      : transfer.feeAmount

  return (
    <div className="divide-y">
      <Row label="Transaction" value={txn} />
      <Row label="Source" value={source} />
      <Row label="Source FX rate" value={transfer.sourceFxRate} />
      <Row label="Destination" value={dest} />
      <Row label="Destination FX rate" value={transfer.destinationFxRate} />
      <Row label="Fee" value={fee} />
    </div>
  )
}
