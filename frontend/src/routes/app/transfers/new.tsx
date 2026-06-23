import { createFileRoute } from '@tanstack/react-router'
import { CreateTransferPage } from '#/components/bank/create-transfer-page'

export const Route = createFileRoute('/app/transfers/new')({
  component: CreateTransferPage,
})
