import type { Route } from './+types/api.auth.login'
import {
  backendLogin,
  buildRefreshSetCookie,
} from '../lib/auth.server'

/**
 * BFF login: Browser → RR → Go auth.
 * Sets HttpOnly refresh cookie on FE origin; returns AT JSON to client memory.
 */
export async function action({ request }: Route.ActionArgs) {
  if (request.method !== 'POST') {
    return new Response('Method Not Allowed', { status: 405 })
  }

  let body: { email?: string; password?: string }
  try {
    body = (await request.json()) as { email?: string; password?: string }
  } catch {
    return Response.json({ message: 'invalid JSON body' }, { status: 400 })
  }

  if (!body.email || !body.password) {
    return Response.json(
      { message: 'email and password are required' },
      { status: 400 },
    )
  }

  try {
    const tokens = await backendLogin(body.email, body.password)
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
      const text = await err.text()
      return new Response(text || 'login failed', { status: err.status })
    }
    return Response.json({ message: 'login failed' }, { status: 500 })
  }
}
