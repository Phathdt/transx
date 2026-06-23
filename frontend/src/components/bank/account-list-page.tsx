import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useListAccounts } from '#/lib/api/generated/wallet/wallet'
import type { DtoAccountListResponse } from '#/lib/api/generated/models'
import type { ApiError } from '#/lib/api/api-error'
import { Card, CardContent } from '#/components/ui/card'
import { Button } from '#/components/ui/button'
import { Skeleton } from '#/components/ui/skeleton'
import { Alert, AlertDescription, AlertTitle } from '#/components/ui/alert'
import { AccountStatusBadge } from './account-status-badge'

const PAGE_SIZE = 20

export function AccountListPage() {
  const [page, setPage] = useState(1)
  const { data, isLoading, isError, error } = useListAccounts<
    DtoAccountListResponse,
    ApiError
  >({
    page,
    pageSize: PAGE_SIZE,
  })

  const accounts = data?.data ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <div className="space-y-6">
      <div>
        <h1 className="display-title text-2xl font-bold text-[var(--sea-ink)]">
          Accounts
        </h1>
        <p className="text-sm text-muted-foreground">
          Your wallet accounts and balances.
        </p>
      </div>

      {isLoading ? (
        <div className="space-y-2">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-16 w-full" />
          ))}
        </div>
      ) : isError ? (
        <Alert variant="destructive">
          <AlertTitle>Could not load accounts</AlertTitle>
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      ) : accounts.length === 0 ? (
        <Card>
          <CardContent className="py-10 text-center text-sm text-muted-foreground">
            No accounts found.
          </CardContent>
        </Card>
      ) : (
        <ul className="space-y-2">
          {accounts.map((account) => (
            <li key={account.accountRef}>
              <Link
                to="/app/accounts/$accountRef"
                params={{ accountRef: account.accountRef ?? '' }}
                className="block no-underline"
              >
                <Card className="transition hover:border-[var(--lagoon-deep)]">
                  <CardContent className="flex items-center justify-between py-4">
                    <div className="min-w-0">
                      <p className="truncate font-medium text-[var(--sea-ink)]">
                        {account.accountRef}
                      </p>
                      <p className="text-sm text-muted-foreground tabular-nums">
                        {account.availableBalance} {account.currency}
                      </p>
                    </div>
                    <AccountStatusBadge status={account.status} />
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
