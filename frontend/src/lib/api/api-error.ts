import type { AxiosError } from 'axios'

/**
 * Normalized API error surfaced to the UI. Mirrors the backend
 * `HandlersErrorResponse` shape ({ error, requestId, traceId }) plus the HTTP
 * status, so components show a safe message and we keep request IDs for debugging.
 */
export interface ApiError {
  message: string
  status: number
  requestId?: string
  traceId?: string
  raw?: unknown
}

interface BackendErrorBody {
  error?: string
  requestId?: string
  traceId?: string
}

function isAxiosError(error: unknown): error is AxiosError<BackendErrorBody> {
  return (
    typeof error === 'object' &&
    error !== null &&
    (error as AxiosError).isAxiosError === true
  )
}

/** Convert any thrown value (Axios error or unknown) into a stable ApiError. */
export function toApiError(error: unknown): ApiError {
  if (isAxiosError(error)) {
    const status = error.response?.status ?? 0
    const body = error.response?.data
    return {
      message: body?.error || error.message || 'Request failed',
      status,
      requestId: body?.requestId,
      traceId: body?.traceId,
      raw: error,
    }
  }

  if (error instanceof Error) {
    return { message: error.message, status: 0, raw: error }
  }

  return { message: 'Unexpected error', status: 0, raw: error }
}
