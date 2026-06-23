import { createFileRoute } from '@tanstack/react-router'
import { AccountDetailPage } from '#/components/bank/account-detail-page'

export const Route = createFileRoute('/app/accounts/$accountRef')({
  component: AccountDetailRoute,
})

function AccountDetailRoute() {
  const { accountRef } = Route.useParams()
  return <AccountDetailPage accountRef={accountRef} />
}
