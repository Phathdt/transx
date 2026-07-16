import { useLoaderData } from 'react-router'
import type { Route } from './+types/app.accounts'
import { AccountListPage } from '#/components/bank/account-list-page'
import { listAccounts } from '../lib/api/generated/wallet/wallet'
import { withServerBearer } from '../lib/server-domain.server'

const PAGE_SIZE = 20
const ALL = 'ALL'

function parsePage(raw: string | null): number {
  const n = Number(raw ?? '1')
  if (!Number.isFinite(n) || n < 1) return 1
  return Math.floor(n)
}

function parseStatus(raw: string | null): string {
  if (!raw || raw === ALL) return ALL
  return raw
}

/**
 * SSR: Node fetches accounts with AT_ssr, then HTML is rendered.
 * Filters/pagination live in the URL so navigation re-runs this loader.
 */
export async function loader({ request }: Route.LoaderArgs) {
  const url = new URL(request.url)
  const page = parsePage(url.searchParams.get('page'))
  const status = parseStatus(url.searchParams.get('status'))
  const currency = url.searchParams.get('currency') ?? ''

  const params = {
    page,
    pageSize: PAGE_SIZE,
    ...(status !== ALL ? { status } : {}),
    ...(currency ? { currency } : {}),
  }

  const accountsRes = await withServerBearer(request, (init) =>
    listAccounts(params, init),
  )

  return {
    accounts: accountsRes.data ?? [],
    total: accountsRes.total ?? 0,
    page: accountsRes.page ?? page,
    pageSize: accountsRes.pageSize ?? PAGE_SIZE,
    status,
    currency,
  }
}

export default function AccountsRoute() {
  const data = useLoaderData<typeof loader>()
  return <AccountListPage {...data} />
}
