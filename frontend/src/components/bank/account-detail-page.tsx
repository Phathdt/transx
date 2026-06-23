import { Link } from '@tanstack/react-router'
import { ArrowLeft } from 'lucide-react'
import { useGetAccount } from '#/lib/api/generated/wallet/wallet'
import type { DtoAccountResponse } from '#/lib/api/generated/models'
import type { ApiError } from '#/lib/api/api-error'
import { Button } from '#/components/ui/button'
import { Card, CardContent } from '#/components/ui/card'
import { Skeleton } from '#/components/ui/skeleton'
import { Alert, AlertDescription, AlertTitle } from '#/components/ui/alert'
import { AccountStatusBadge } from './account-status-badge'

function Row({ label, value }: { label: string; value?: string }) {
  if (!value) return null
  return (
    <div className="flex items-center justify-between py-2.5 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-medium tabular-nums text-[var(--sea-ink)]">
        {value}
      </span>
    </div>
  )
}

export function AccountDetailPage({ accountRef }: { accountRef: string }) {
  const { data, isLoading, isError, error } = useGetAccount<
    DtoAccountResponse,
    ApiError
  >(accountRef, { query: { enabled: Boolean(accountRef) } })

  return (
    <div className="mx-auto max-w-xl space-y-4">
      <Button asChild variant="ghost" size="sm">
        <Link to="/app/accounts">
          <ArrowLeft className="size-4" />
          Back to accounts
        </Link>
      </Button>

      {isLoading ? (
        <Skeleton className="h-64 w-full rounded-2xl" />
      ) : isError ? (
        <Alert variant="destructive">
          <AlertTitle>Could not load account</AlertTitle>
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      ) : data ? (
        <Card className="glass-card overflow-hidden border-0 p-0 shadow-none">
          <div className="hero-band rounded-none border-0 px-6 pt-6 pb-7">
            <div className="flex items-start justify-between gap-3">
              <p className="island-kicker">Available balance</p>
              <AccountStatusBadge status={data.status} />
            </div>
            <p className="amount-display mt-3 text-4xl font-bold sm:text-5xl">
              {data.availableBalance ?? '—'}{' '}
              {data.currency ? (
                <span className="unit-chip ml-1 px-2.5 py-1 text-sm align-middle">
                  {data.currency}
                </span>
              ) : null}
            </p>
            <p className="mt-3 truncate font-mono text-xs text-muted-foreground">
              {data.accountRef}
            </p>
          </div>
          <CardContent className="pt-5 pb-6">
            <div className="divide-y divide-[var(--line)]">
              <Row label="Currency" value={data.currency} />
              <Row
                label="Hold balance"
                value={
                  data.holdBalance
                    ? `${data.holdBalance} ${data.currency ?? ''}`.trim()
                    : undefined
                }
              />
            </div>
            <Button asChild className="mt-5 w-full">
              <Link to="/app/transfers/new">
                New transfer from this account
              </Link>
            </Button>
          </CardContent>
        </Card>
      ) : null}
    </div>
  )
}
