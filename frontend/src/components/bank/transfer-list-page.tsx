import { useMemo, useState } from 'react'
import { Link } from 'react-router'
import { ArrowDownLeft, ArrowUpRight, ChevronRight, Plus } from 'lucide-react'
import {
  useListAccounts,
  useListTransfers,
} from '#/lib/api/generated/wallet/wallet'
import type { DtoTransferListResponse } from '#/lib/api/generated/models'
import type { ApiError } from '#/lib/api/api-error'
import {
  transferCounterparty,
  transferDirection,
} from '#/lib/transfer/transfer-direction'
import { Button } from '#/components/ui/button'
import { Card, CardContent } from '#/components/ui/card'
import { Label } from '#/components/ui/label'
import { Skeleton } from '#/components/ui/skeleton'
import { Alert, AlertDescription, AlertTitle } from '#/components/ui/alert'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'
import { TransferStatusBadge } from './transfer-status-badge'

const PAGE_SIZE = 20

// Sentinel for the "all statuses" option; an empty string is not a valid
// SelectItem value, so map it to/from "" before calling the API.
const ALL = 'ALL'

const TRANSFER_STATUSES = [
  'PENDING',
  'RESERVED',
  'PROCESSING',
  'SUBMITTED',
  'SUCCEEDED',
  'FAILED',
  'REVERSED',
  'UNKNOWN',
]

export function TransferListPage() {
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState(ALL)
  const [accountRef, setAccountRef] = useState('')

  // The caller's accounts populate the account filter options.
  const { data: accountsData } = useListAccounts({ pageSize: 100 })
  const accounts = useMemo(() => accountsData?.data ?? [], [accountsData])
  const ownedRefs = useMemo(
    () =>
      new Set(
        accounts
          .map((a) => a.accountRef)
          .filter((r): r is string => Boolean(r)),
      ),
    [accounts],
  )

  const { data, isLoading, isError, error } = useListTransfers<
    DtoTransferListResponse,
    ApiError
  >({
    page,
    pageSize: PAGE_SIZE,
    ...(status !== ALL ? { status } : {}),
    ...(accountRef ? { accountRef } : {}),
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

      <Card className="glass-card border-0 shadow-none">
        <CardContent className="flex flex-wrap items-end gap-4 py-4">
          <div className="space-y-1.5">
            <Label htmlFor="status-filter">Status</Label>
            <Select
              value={status}
              onValueChange={(v) => {
                setStatus(v)
                setPage(1)
              }}
            >
              <SelectTrigger id="status-filter" className="w-44">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>All statuses</SelectItem>
                {TRANSFER_STATUSES.map((s) => (
                  <SelectItem key={s} value={s}>
                    {s}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="account-filter">Account</Label>
            <Select
              value={accountRef || ALL}
              onValueChange={(v) => {
                setAccountRef(v === ALL ? '' : v)
                setPage(1)
              }}
            >
              <SelectTrigger id="account-filter" className="w-56">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>All accounts</SelectItem>
                {accounts.map((acc) => (
                  <SelectItem key={acc.accountRef} value={acc.accountRef ?? ''}>
                    {acc.accountRef} · {acc.currency}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {status !== ALL || accountRef ? (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setStatus(ALL)
                setAccountRef('')
                setPage(1)
              }}
            >
              Clear
            </Button>
          ) : null}
        </CardContent>
      </Card>

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
            <p className="text-sm text-muted-foreground">No transfers yet.</p>
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
          {transfers.map((transfer) => {
            const direction = transferDirection(transfer, ownedRefs)
            const counterparty = transferCounterparty(transfer, direction)
            const received = direction === 'received'
            const directionLabel =
              direction === 'received'
                ? 'Received'
                : direction === 'self'
                  ? 'Own transfer'
                  : 'Sent'
            return (
              <li key={transfer.transferId}>
                <Link
                  to={`/app/transfers/${transfer.transferId ?? ''}`}
                  className="list-row flex items-center gap-4 px-4 py-3.5 no-underline sm:px-5"
                >
                  <span className="row-avatar size-11 shrink-0">
                    {received ? (
                      <ArrowDownLeft className="size-5" strokeWidth={2.2} />
                    ) : (
                      <ArrowUpRight className="size-5" strokeWidth={2.2} />
                    )}
                  </span>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium text-[var(--sea-ink)]">
                      {directionLabel}
                      {counterparty ? (
                        <span className="text-muted-foreground">
                          {' · '}
                          {counterparty}
                        </span>
                      ) : null}
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
            )
          })}
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
