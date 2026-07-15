import type { Route } from './+types/api.auth.refresh'
import {
  backendRefresh,
  buildRefreshSetCookie,
  getRefreshTokenFromRequest,
} from '../lib/auth.server'

/**
 * BFF silent refresh: read RT cookie → Go /refresh → new cookie + AT JSON.
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
    const tokens = await backendRefresh(refreshToken)
    return Response.json(
      {
        accessToken: tokens.accessToken,
        tokenType: tokens.tokenType ?? 'Bearer',
        userId: tokens.userId,
        userName: tokens.userName,
      },
      {
        headers: {
          'Set-Cookie': buildRefreshSetCookie(tokens.refreshToken),
        },
      },
    )
  } catch (err) {
    if (err instanceof Response) {
      return Response.json({ message: 'refresh failed' }, { status: err.status })
    }
    return Response.json({ message: 'refresh failed' }, { status: 500 })
  }
}
