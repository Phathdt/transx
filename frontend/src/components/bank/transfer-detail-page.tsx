import { useMemo, useState } from 'react'
import { Link } from 'react-router'
import { ArrowLeft, Loader2 } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import {
  getGetTransferQueryKey,
  useCancelTransfer,
  useListAccounts,
} from '#/lib/api/generated/wallet/wallet'
import { useTransferStatusPolling } from '#/hooks/use-transfer-status-polling'
import {
  formatFailureReason,
  isTerminalStatus,
} from '#/lib/transfer/transfer-status'
import { transferDirection } from '#/lib/transfer/transfer-direction'
import { toApiError } from '#/lib/api/api-error'
import { Button } from '#/components/ui/button'
import { Card, CardContent } from '#/components/ui/card'
import { Skeleton } from '#/components/ui/skeleton'
import { Alert, AlertDescription, AlertTitle } from '#/components/ui/alert'
import { TransferStatusBadge } from './transfer-status-badge'
import { TransferMoneySummary } from './transfer-money-summary'

export function TransferDetailPage({ transferId }: { transferId: string }) {
  const queryClient = useQueryClient()
  const [cancelError, setCancelError] = useState<string | null>(null)
  const { data, isLoading, isError, error } =
    useTransferStatusPolling(transferId)

  const cancelMutation = useCancelTransfer({
    mutation: {
      onSuccess: () => {
        toast.success('Transfer cancelled')
        queryClient.invalidateQueries({
          queryKey: getGetTransferQueryKey(transferId),
        })
      },
      onError: (err) => {
        setCancelError(toApiError(err).message)
      },
    },
  })

  function handleCancel() {
    setCancelError(null)
    cancelMutation.mutate({ transferId })
  }

  const { data: accountsData } = useListAccounts({ pageSize: 100 })
  const ownedRefs = useMemo(
    () =>
      new Set(
        (accountsData?.data ?? [])
          .map((a) => a.accountRef)
          .filter((r): r is string => Boolean(r)),
      ),
    [accountsData],
  )
  const direction = data ? transferDirection(data, ownedRefs) : 'sent'
  const directionLabel =
    direction === 'received'
      ? 'Received'
      : direction === 'self'
        ? 'Own transfer'
        : 'Sent'

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
              <p className="island-kicker">{directionLabel}</p>
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

            {status === 'SCHEDULED' && data.executeAt ? (
              <p className="rounded-xl bg-[var(--chip-bg)] px-3 py-2 text-sm text-muted-foreground">
                Scheduled to run on{' '}
                {new Date(data.executeAt).toLocaleString()}
              </p>
            ) : null}

            {cancelError ? (
              <Alert variant="destructive">
                <AlertTitle>Could not cancel transfer</AlertTitle>
                <AlertDescription>{cancelError}</AlertDescription>
              </Alert>
            ) : null}

            <TransferMoneySummary transfer={data} direction={direction} />

            {status === 'SCHEDULED' ? (
              <Button
                variant="outline"
                className="w-full"
                disabled={cancelMutation.isPending}
                onClick={handleCancel}
              >
                {cancelMutation.isPending
                  ? 'Cancelling…'
                  : 'Cancel scheduled transfer'}
              </Button>
            ) : null}
          </CardContent>
        </Card>
      ) : null}
    </div>
  )
}
