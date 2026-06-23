import { Link } from '@tanstack/react-router'
import { ArrowLeft, Loader2 } from 'lucide-react'
import { useTransferStatusPolling } from '#/hooks/use-transfer-status-polling'
import {
  formatFailureReason,
  isTerminalStatus,
} from '#/lib/transfer/transfer-status'
import { Button } from '#/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '#/components/ui/card'
import { Skeleton } from '#/components/ui/skeleton'
import { Separator } from '#/components/ui/separator'
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
        <Skeleton className="h-64 w-full" />
      ) : isError ? (
        <Alert variant="destructive">
          <AlertTitle>Could not load transfer</AlertTitle>
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      ) : data ? (
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <CardTitle className="truncate">{data.transferId}</CardTitle>
              <TransferStatusBadge status={status} />
            </div>
          </CardHeader>
          <CardContent className="space-y-4">
            {polling ? (
              <p className="flex items-center gap-2 text-sm text-muted-foreground">
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

            <Separator />
            <TransferMoneySummary transfer={data} />
          </CardContent>
        </Card>
      ) : null}
    </div>
  )
}
