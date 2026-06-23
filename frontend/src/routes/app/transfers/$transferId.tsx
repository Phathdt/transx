import { createFileRoute } from '@tanstack/react-router'
import { TransferDetailPage } from '#/components/bank/transfer-detail-page'

export const Route = createFileRoute('/app/transfers/$transferId')({
  component: TransferDetailRoute,
})

function TransferDetailRoute() {
  const { transferId } = Route.useParams()
  return <TransferDetailPage transferId={transferId} />
}
