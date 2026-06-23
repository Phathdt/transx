import { Badge } from '#/components/ui/badge'

// Account status enum from the backend: ACTIVE, FROZEN, CLOSED.
const VARIANT: Record<
  string,
  'default' | 'secondary' | 'destructive' | 'outline'
> = {
  ACTIVE: 'default',
  FROZEN: 'outline',
  CLOSED: 'destructive',
}

export function AccountStatusBadge({ status }: { status?: string }) {
  if (!status) return null
  return <Badge variant={VARIANT[status] ?? 'secondary'}>{status}</Badge>
}
