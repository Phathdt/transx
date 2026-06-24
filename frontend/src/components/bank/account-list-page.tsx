import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { ChevronRight, Wallet } from 'lucide-react'
import { useListAccounts } from '#/lib/api/generated/wallet/wallet'
import type { DtoAccountListResponse } from '#/lib/api/generated/models'
import type { ApiError } from '#/lib/api/api-error'
import { Card, CardContent } from '#/components/ui/card'
import { Button } from '#/components/ui/button'
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
import { AccountStatusBadge } from './account-status-badge'

const PAGE_SIZE = 20

// Sentinel for the "all statuses" option; an empty string is not a valid
// SelectItem value, so map it to/from "" before calling the API.
const ALL = 'ALL'

const ACCOUNT_STATUSES = ['ACTIVE', 'FROZEN', 'CLOSED']

// ISO-4217 allow-list mirrored from the backend (services/currency.go).
const CURRENCIES = ['USD', 'EUR', 'VND']

export function AccountListPage() {
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState(ALL)
  const [currency, setCurrency] = useState('')

  const { data, isLoading, isError, error } = useListAccounts<
    DtoAccountListResponse,
    ApiError
  >({
    page,
    pageSize: PAGE_SIZE,
    ...(status !== ALL ? { status } : {}),
    ...(currency ? { currency } : {}),
  })

  const accounts = data?.data ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <div className="space-y-7">
      <div>
        <p className="island-kicker mb-1">Wallets</p>
        <h1 className="display-title text-3xl font-bold text-[var(--sea-ink)]">
          Accounts
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Your wallet accounts and balances.
        </p>
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
                {ACCOUNT_STATUSES.map((s) => (
                  <SelectItem key={s} value={s}>
                    {s}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="currency-filter">Currency</Label>
            <Select
              value={currency || ALL}
              onValueChange={(v) => {
                setCurrency(v === ALL ? '' : v)
                setPage(1)
              }}
            >
              <SelectTrigger id="currency-filter" className="w-36">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>All</SelectItem>
                {CURRENCIES.map((c) => (
                  <SelectItem key={c} value={c}>
                    {c}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {status !== ALL || currency ? (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setStatus(ALL)
                setCurrency('')
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
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-[72px] w-full rounded-2xl" />
          ))}
        </div>
      ) : isError ? (
        <Alert variant="destructive">
          <AlertTitle>Could not load accounts</AlertTitle>
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      ) : accounts.length === 0 ? (
        <Card className="glass-card border-0 shadow-none">
          <CardContent className="flex flex-col items-center gap-3 py-14 text-center">
            <span className="row-avatar size-12">
              <Wallet className="size-5" />
            </span>
            <p className="text-sm text-muted-foreground">No accounts found.</p>
          </CardContent>
        </Card>
      ) : (
        <ul className="space-y-3">
          {accounts.map((account) => (
            <li key={account.accountRef}>
              <Link
                to="/app/accounts/$accountRef"
                params={{ accountRef: account.accountRef ?? '' }}
                className="list-row flex items-center gap-4 px-4 py-3.5 no-underline sm:px-5"
              >
                <span className="row-avatar size-11 shrink-0 text-sm font-bold uppercase">
                  {(account.accountRef ?? '?').slice(0, 2)}
                </span>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium text-[var(--sea-ink)]">
                    {account.accountRef}
                  </p>
                  <p className="mt-0.5 text-lg font-semibold tabular-nums text-[var(--sea-ink)]">
                    {account.availableBalance}{' '}
                    <span className="text-sm font-medium text-muted-foreground">
                      {account.currency}
                    </span>
                  </p>
                </div>
                <div className="flex shrink-0 items-center gap-3">
                  <AccountStatusBadge status={account.status} />
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
