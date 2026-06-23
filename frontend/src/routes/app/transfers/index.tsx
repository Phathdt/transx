import { createFileRoute } from '@tanstack/react-router'
import { TransferListPage } from '#/components/bank/transfer-list-page'

export const Route = createFileRoute('/app/transfers/')({
  component: TransferListPage,
})
