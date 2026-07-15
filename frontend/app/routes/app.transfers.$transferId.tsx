import { useParams } from 'react-router'
import { TransferDetailPage } from '#/components/bank/transfer-detail-page'

export default function TransferDetailRoute() {
  const { transferId } = useParams()
  return <TransferDetailPage transferId={transferId ?? ''} />
}
