import { Link } from '@tanstack/react-router'
import { ArrowLeft } from 'lucide-react'
import { useGetAccount } from '#/lib/api/generated/wallet/wallet'
import type { DtoAccountResponse } from '#/lib/api/generated/models'
import type { ApiError } from '#/lib/api/api-error'
import { Button } from '#/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '#/components/ui/card'
import { Skeleton } from '#/components/ui/skeleton'
import { Separator } from '#/components/ui/separator'
import { Alert, AlertDescription, AlertTitle } from '#/components/ui/alert'
import { AccountStatusBadge } from './account-status-badge'

function Row({ label, value }: { label: string; value?: string }) {
  if (!value) return null
  return (
    <div className="flex items-center justify-between py-1.5 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-medium tabular-nums">{value}</span>
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
        <Skeleton className="h-56 w-full" />
      ) : isError ? (
        <Alert variant="destructive">
          <AlertTitle>Could not load account</AlertTitle>
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      ) : data ? (
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <CardTitle className="truncate">{data.accountRef}</CardTitle>
              <AccountStatusBadge status={data.status} />
            </div>
          </CardHeader>
          <CardContent>
            <div className="divide-y">
              <Row label="Currency" value={data.currency} />
              <Row
                label="Available balance"
                value={
                  data.availableBalance
                    ? `${data.availableBalance} ${data.currency ?? ''}`.trim()
                    : undefined
                }
              />
              <Row
                label="Hold balance"
                value={
                  data.holdBalance
                    ? `${data.holdBalance} ${data.currency ?? ''}`.trim()
                    : undefined
                }
              />
            </div>
            <Separator className="my-4" />
            <Button asChild className="w-full">
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
