import type { Route } from './+types/api.auth.login'
import { z } from 'zod'
import {
  backendLogin,
  backendSessionAccess,
  buildRefreshSetCookie,
  parseSessionId,
  putServerAccessToken,
} from '../lib/auth.server'

const LoginBodySchema = z.object({
  email: z.string().min(1, 'email is required'),
  password: z.string().min(1, 'password is required'),
})

/**
 * BFF login: Browser → RR → Go auth.
 * Dual AT: login yields AT_browser + RT; /session/access yields AT_ssr (cached).
 * Cookie = login RT (unrotated). Browser JSON = AT_browser only.
 */
export async function action({ request }: Route.ActionArgs) {
  if (request.method !== 'POST') {
    return new Response('Method Not Allowed', { status: 405 })
  }

  let body: z.infer<typeof LoginBodySchema>
  try {
    const json: unknown = await request.json()
    const result = LoginBodySchema.safeParse(json)
    if (!result.success) {
      return Response.json(
        { message: 'email and password are required' },
        { status: 400 },
      )
    }
    body = result.data
  } catch {
    return Response.json({ message: 'invalid JSON body' }, { status: 400 })
  }

  try {
    const pair = await backendLogin(body.email, body.password)
    // Fail closed: if SSR AT mint fails, do not Set-Cookie.
    const ssr = await backendSessionAccess(pair.refreshToken)
    const sid = parseSessionId(pair.refreshToken)
    if (sid) {
      await putServerAccessToken(sid, ssr.accessToken)
    }

    return Response.json(
      {
        accessToken: pair.accessToken,
        tokenType: pair.tokenType ?? 'Bearer',
        userId: pair.userId,
        userName: pair.userName,
      },
      {
        headers: {
          'Set-Cookie': buildRefreshSetCookie(pair.refreshToken),
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
