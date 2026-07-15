import { queryOptions } from '@tanstack/react-query'
import { listAccounts } from '#/lib/api/generated/wallet/wallet'
import type { ApiError } from '#/lib/api/api-error'

/** Stable key for the auth-check query (server truth for a valid token). */
export const authCheckQueryKey = ['auth', 'check'] as const

/**
 * Query options that confirm the current token is valid. The gateway's
 * `/check` route is an internal ForwardAuth address, not reachable from the
 * browser, so we probe a real gated endpoint instead: a 1-item `GET /accounts`
 * returns 200 for a valid token and 401 otherwise. Retry is disabled so an
 * invalid token fails fast instead of looping.
 */
export const authCheckQueryOptions = () =>
  queryOptions<unknown, ApiError>({
    queryKey: authCheckQueryKey,
    queryFn: ({ signal }) => listAccounts({ pageSize: 1 }, undefined, signal),
    retry: false,
    staleTime: 5 * 60 * 1000,
  })
