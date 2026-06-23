import { Link } from '@tanstack/react-router'
import { ArrowLeft, Loader2 } from 'lucide-react'
import { useTransferStatusPolling } from '#/hooks/use-transfer-status-polling'
import {
  formatFailureReason,
  isTerminalStatus,
} from '#/lib/transfer/transfer-status'
import { Button } from '#/components/ui/button'
import { Card, CardContent } from '#/components/ui/card'
import { Skeleton } from '#/components/ui/skeleton'
import { Alert, AlertDescription, AlertTitle } from '#/components/ui/alert'
import { TransferStatusBadge } from './transfer-status-badge'
import { TransferMoneySummary } from './transfer-money-summary'

export function TransferDetailPage({ transferId }: { transferId: string }) {
  const { data, isLoading, isError, error } =
    useTransferStatusPolling(transferId)

  const status = data?.status
  const polling = Boolean(status) && !isTerminalStatus(status)
  const failure = formatFailureReason(data?.failureReason)

  return (
    <div className="mx-auto max-w-xl space-y-4">
      <Button asChild variant="ghost" size="sm">
        <Link to="/app/transfers">
          <ArrowLeft className="size-4" />
          Back to transfers
        </Link>
      </Button>

      {isLoading ? (
        <Skeleton className="h-72 w-full rounded-2xl" />
      ) : isError ? (
        <Alert variant="destructive">
          <AlertTitle>Could not load transfer</AlertTitle>
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      ) : data ? (
        <Card className="glass-card overflow-hidden border-0 p-0 shadow-none">
          <div className="hero-band rounded-none border-0 px-6 pt-6 pb-7">
            <div className="flex items-start justify-between gap-3">
              <p className="island-kicker">Transfer</p>
              <TransferStatusBadge status={status} />
            </div>
            <p className="amount-display mt-3 text-4xl font-bold sm:text-5xl">
              {data.transactionAmount}{' '}
              <span className="unit-chip ml-1 px-2.5 py-1 text-sm align-middle">
                {data.transactionCurrency}
              </span>
            </p>
            <p className="mt-3 truncate font-mono text-xs text-muted-foreground">
              {data.transferId}
            </p>
          </div>
          <CardContent className="space-y-4 pt-5 pb-6">
            {polling ? (
              <p className="flex items-center gap-2 rounded-xl bg-[var(--chip-bg)] px-3 py-2 text-sm text-muted-foreground">
                <Loader2 className="size-4 animate-spin" />
                Processing — this page updates automatically.
              </p>
            ) : null}

            {failure ? (
              <Alert variant="destructive">
                <AlertTitle>Transfer not completed</AlertTitle>
                <AlertDescription>{failure}</AlertDescription>
              </Alert>
            ) : null}

            {status === 'UNKNOWN' ? (
              <Alert>
                <AlertTitle>Status unknown</AlertTitle>
                <AlertDescription>
                  We could not confirm the final state. Check back later before
                  retrying.
                </AlertDescription>
              </Alert>
            ) : null}

            <TransferMoneySummary transfer={data} />
          </CardContent>
        </Card>
      ) : null}
    </div>
  )
}
