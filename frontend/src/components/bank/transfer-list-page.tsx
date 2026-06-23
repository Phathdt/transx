import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { ArrowUpRight, ChevronRight, Plus } from 'lucide-react'
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
    <div className="space-y-7">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="island-kicker mb-1">Activity</p>
          <h1 className="display-title text-3xl font-bold text-[var(--sea-ink)]">
            Transfers
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
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
        <div className="space-y-3">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-[72px] w-full rounded-2xl" />
          ))}
        </div>
      ) : isError ? (
        <Alert variant="destructive">
          <AlertTitle>Could not load transfers</AlertTitle>
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      ) : transfers.length === 0 ? (
        <Card className="glass-card border-0 shadow-none">
          <CardContent className="flex flex-col items-center gap-3 py-14 text-center">
            <span className="row-avatar size-12">
              <ArrowUpRight className="size-5" />
            </span>
            <p className="text-sm text-muted-foreground">
              No transfers yet.
            </p>
            <Button asChild size="sm">
              <Link to="/app/transfers/new">
                <Plus className="size-4" />
                Create your first transfer
              </Link>
            </Button>
          </CardContent>
        </Card>
      ) : (
        <ul className="space-y-3">
          {transfers.map((transfer) => (
            <li key={transfer.transferId}>
              <Link
                to="/app/transfers/$transferId"
                params={{ transferId: transfer.transferId ?? '' }}
                className="list-row flex items-center gap-4 px-4 py-3.5 no-underline sm:px-5"
              >
                <span className="row-avatar size-11 shrink-0">
                  <ArrowUpRight className="size-5" strokeWidth={2.2} />
                </span>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium text-[var(--sea-ink)]">
                    {transfer.transferId}
                  </p>
                  <p className="mt-0.5 text-lg font-semibold tabular-nums text-[var(--sea-ink)]">
                    {transfer.transactionAmount}{' '}
                    <span className="text-sm font-medium text-muted-foreground">
                      {transfer.transactionCurrency}
                    </span>
                  </p>
                </div>
                <div className="flex shrink-0 items-center gap-3">
                  <TransferStatusBadge status={transfer.status} />
                  <ChevronRight className="row-chevron size-5" />
                </div>
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
