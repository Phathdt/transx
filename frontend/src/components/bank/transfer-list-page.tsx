import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { Plus } from 'lucide-react'
import { useListTransfers } from '#/lib/api/generated/wallet/wallet'
import type { DtoTransferListResponse } from '#/lib/api/generated/models'
import type { ApiError } from '#/lib/api/api-error'
import { Button } from '#/components/ui/button'
import { Card, CardContent } from '#/components/ui/card'
import { Skeleton } from '#/components/ui/skeleton'
import { Alert, AlertDescription, AlertTitle } from '#/components/ui/alert'
import { TransferStatusBadge } from './transfer-status-badge'

const PAGE_SIZE = 20

export function TransferListPage() {
  const [page, setPage] = useState(1)
  const { data, isLoading, isError, error } = useListTransfers<
    DtoTransferListResponse,
    ApiError
  >({
    page,
    pageSize: PAGE_SIZE,
  })

  const transfers = data?.data ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="display-title text-2xl font-bold text-[var(--sea-ink)]">
            Transfers
          </h1>
          <p className="text-sm text-muted-foreground">
            Your transfer history, newest first.
          </p>
        </div>
        <Button asChild>
          <Link to="/app/transfers/new">
            <Plus className="size-4" />
            New Transfer
          </Link>
        </Button>
      </div>

      {isLoading ? (
        <div className="space-y-2">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-16 w-full" />
          ))}
        </div>
      ) : isError ? (
        <Alert variant="destructive">
          <AlertTitle>Could not load transfers</AlertTitle>
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      ) : transfers.length === 0 ? (
        <Card>
          <CardContent className="py-10 text-center text-sm text-muted-foreground">
            No transfers yet.{' '}
            <Link to="/app/transfers/new" className="font-medium">
              Create your first transfer
            </Link>
            .
          </CardContent>
        </Card>
      ) : (
        <ul className="space-y-2">
          {transfers.map((transfer) => (
            <li key={transfer.transferId}>
              <Link
                to="/app/transfers/$transferId"
                params={{ transferId: transfer.transferId ?? '' }}
                className="block no-underline"
              >
                <Card className="transition hover:border-[var(--lagoon-deep)]">
                  <CardContent className="flex items-center justify-between py-4">
                    <div className="min-w-0">
                      <p className="truncate font-medium text-[var(--sea-ink)]">
                        {transfer.transferId}
                      </p>
                      <p className="text-sm text-muted-foreground tabular-nums">
                        {transfer.transactionAmount}{' '}
                        {transfer.transactionCurrency}
                      </p>
                    </div>
                    <TransferStatusBadge status={transfer.status} />
                  </CardContent>
                </Card>
              </Link>
            </li>
          ))}
        </ul>
      )}

      {total > PAGE_SIZE ? (
        <div className="flex items-center justify-between text-sm">
          <Button
            variant="outline"
            size="sm"
            disabled={page <= 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
          >
            Previous
          </Button>
          <span className="text-muted-foreground">
            Page {page} of {totalPages}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={page >= totalPages}
            onClick={() => setPage((p) => p + 1)}
          >
            Next
          </Button>
        </div>
      ) : null}
    </div>
  )
}
