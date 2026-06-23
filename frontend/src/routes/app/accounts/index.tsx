import { createFileRoute } from '@tanstack/react-router'
import { AccountListPage } from '#/components/bank/account-list-page'

export const Route = createFileRoute('/app/accounts/')({
  component: AccountListPage,
})
