import type { Route } from './+types/api.auth.refresh'
import {
  backendSessionAccess,
  getRefreshTokenFromRequest,
} from '../lib/auth.server'

/**
 * BFF silent AT renew (route name kept for FE compat).
 * Cookie RT → Go POST /session/access → new AT only.
 * No Set-Cookie (RT unchanged). Does NOT call Go /refresh.
 */
export async function action({ request }: Route.ActionArgs) {
  if (request.method !== 'POST') {
    return new Response('Method Not Allowed', { status: 405 })
  }

  const refreshToken = getRefreshTokenFromRequest(request)
  if (!refreshToken) {
    return Response.json({ message: 'missing refresh session' }, { status: 401 })
  }

  try {
    const access = await backendSessionAccess(refreshToken)
    return Response.json({
      accessToken: access.accessToken,
      tokenType: access.tokenType ?? 'Bearer',
      userId: access.userId,
      userName: access.userName,
    })
  } catch (err) {
    if (err instanceof Response) {
      return Response.json(
        { message: 'refresh failed' },
        { status: err.status },
      )
    }
    return Response.json({ message: 'refresh failed' }, { status: 500 })
  }
}
