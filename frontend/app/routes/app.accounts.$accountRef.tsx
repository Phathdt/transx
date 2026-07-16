import { useParams } from 'react-router'
import { AccountDetailPage } from '#/components/bank/account-detail-page'

export default function AccountDetailRoute() {
  const { accountRef } = useParams()
  return <AccountDetailPage accountRef={accountRef ?? ''} />
}