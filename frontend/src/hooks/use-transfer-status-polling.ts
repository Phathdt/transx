import { useGetTransfer } from '#/lib/api/generated/wallet/wallet'
import type { DtoTransferResponse } from '#/lib/api/generated/models'
import type { ApiError } from '#/lib/api/api-error'
import { isTerminalStatus } from '#/lib/transfer/transfer-status'

const POLL_INTERVAL_MS = 2500

/**
 * Loads a transfer and polls while its status is non-terminal. Polling stops
 * automatically once the transfer reaches a terminal state.
 */
export function useTransferStatusPolling(transferId: string) {
  return useGetTransfer<DtoTransferResponse, ApiError>(transferId, {
    query: {
      enabled: Boolean(transferId),
      refetchInterval: (query) => {
        const status = query.state.data?.status
        return isTerminalStatus(status) ? false : POLL_INTERVAL_MS
      },
    },
  })
}
