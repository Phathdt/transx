import { Badge } from '#/components/ui/badge'
import { isTerminalStatus } from '#/lib/transfer/transfer-status'

const SUCCESS = new Set(['SUCCEEDED'])
const DANGER = new Set(['FAILED', 'REVERSED', 'UNKNOWN'])
// CANCELLED is a deliberate user action, not a failure — shown neutral like
// the in-flight statuses rather than destructive.
const NEUTRAL_TERMINAL = new Set(['CANCELLED'])

export function TransferStatusBadge({ status }: { status?: string }) {
  if (!status) return null

  let variant: 'default' | 'secondary' | 'destructive' | 'outline' = 'secondary'
  if (SUCCESS.has(status)) variant = 'default'
  else if (DANGER.has(status)) variant = 'destructive'
  else if (NEUTRAL_TERMINAL.has(status) || !isTerminalStatus(status))
    variant = 'outline'

  return <Badge variant={variant}>{status}</Badge>
}
