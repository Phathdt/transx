import type { Route } from './+types/api.auth.logout'
import {
  backendLogout,
  buildRefreshClearCookie,
  clearServerAccessTokenForRefresh,
  getRefreshTokenFromRequest,
} from '../lib/auth.server'

/**
 * BFF logout: revoke this RT at Go auth + clear cookie + DEL this rr:at key.
 */
export async function action({ request }: Route.ActionArgs) {
  if (request.method !== 'POST') {
    return new Response('Method Not Allowed', { status: 405 })
  }

  const refreshToken = getRefreshTokenFromRequest(request)
  await backendLogout(refreshToken)
  await clearServerAccessTokenForRefresh(refreshToken)

  return new Response(null, {
    status: 204,
    headers: {
      'Set-Cookie': buildRefreshClearCookie(),
    },
  })
}
