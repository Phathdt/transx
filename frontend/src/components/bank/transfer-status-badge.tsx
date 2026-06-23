import { Badge } from '#/components/ui/badge'
import { isTerminalStatus } from '#/lib/transfer/transfer-status'

const SUCCESS = new Set(['SUCCEEDED'])
const DANGER = new Set(['FAILED', 'REVERSED', 'UNKNOWN'])

export function TransferStatusBadge({ status }: { status?: string }) {
  if (!status) return null

  let variant: 'default' | 'secondary' | 'destructive' | 'outline' = 'secondary'
  if (SUCCESS.has(status)) variant = 'default'
  else if (DANGER.has(status)) variant = 'destructive'
  else if (!isTerminalStatus(status)) variant = 'outline'

  return <Badge variant={variant}>{status}</Badge>
}
