import { useLoaderData } from 'react-router'
import type { Route } from './+types/app.transfers'
import { TransferListPage } from '#/components/bank/transfer-list-page'
import {
  listAccounts,
  listTransfers,
} from '../lib/api/generated/wallet/wallet'
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
 * SSR: Node fetches transfers + accounts with AT_ssr, then HTML is rendered.
 * Filters/pagination live in the URL so navigation re-runs this loader.
 */
export async function loader({ request }: Route.LoaderArgs) {
  const url = new URL(request.url)
  const page = parsePage(url.searchParams.get('page'))
  const status = parseStatus(url.searchParams.get('status'))
  const accountRef = url.searchParams.get('accountRef') ?? ''

  const transferParams = {
    page,
    pageSize: PAGE_SIZE,
    ...(status !== ALL ? { status } : {}),
    ...(accountRef ? { accountRef } : {}),
  }

  const { transfers, accounts } = await withServerBearer(request, async (init) => {
    const [transfersRes, accountsRes] = await Promise.all([
      listTransfers(transferParams, init),
      listAccounts({ pageSize: 100 }, init),
    ])
    return { transfers: transfersRes, accounts: accountsRes }
  })

  return {
    transfers: transfers.data ?? [],
    total: transfers.total ?? 0,
    page: transfers.page ?? page,
    pageSize: transfers.pageSize ?? PAGE_SIZE,
    status,
    accountRef,
    accounts: accounts.data ?? [],
  }
}

export default function TransfersRoute() {
  const data = useLoaderData<typeof loader>()
  return <TransferListPage {...data} />
}
