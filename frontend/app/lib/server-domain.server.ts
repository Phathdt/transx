/**
 * Helpers for RR loaders that call Go domain APIs with AT_ssr.
 * Never import from client bundles.
 */

import { redirect } from 'react-router'
import {
  clearServerAccessTokenForRefresh,
  getRefreshTokenFromRequest,
  getServerAccessToken,
} from './auth.server'

/** Resolve Authorization header for server-side domain fetches. */
export async function requireBearerInit(request: Request): Promise<RequestInit> {
  const token = await getServerAccessToken(request)
  if (!token) throw redirect('/login')
  return {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  }
}

/**
 * Run a domain call with AT_ssr. On 401 (stale cached JWT), drop rr:at,
 * remint once, retry; still 401 → login.
 */
export async function withServerBearer<T>(
  request: Request,
  call: (init: RequestInit) => Promise<T>,
): Promise<T> {
  const run = async (): Promise<T> => {
    const init = await requireBearerInit(request)
    return call(init)
  }

  try {
    return await run()
  } catch (err) {
    if (!(err instanceof Response) || err.status !== 401) throw err

    const rt = getRefreshTokenFromRequest(request)
    await clearServerAccessTokenForRefresh(rt)

    try {
      return await run()
    } catch (retryErr) {
      if (retryErr instanceof Response && retryErr.status === 401) {
        throw redirect('/login')
      }
      throw retryErr
    }
  }
}
