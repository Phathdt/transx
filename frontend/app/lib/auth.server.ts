/**
 * React Router BFF auth helpers.
 * Owns the HttpOnly refresh-token cookie. Go auth only speaks JSON tokens.
 */

const REFRESH_COOKIE = 'refresh_token'
const DEFAULT_MAX_AGE = 60 * 60 * 24 * 30 // 30d

export type TokenPair = {
  accessToken: string
  refreshToken: string
  tokenType?: string
  userId?: string
  userName?: string
}

export function backendAuthBaseURL(): string {
  // Server-side calls to Go auth through Traefik (or direct auth service).
  return (
    process.env.AUTH_API_BASE_URL ??
    process.env.VITE_API_BASE_URL ??
    'http://localhost:4000/api/v1'
  )
}

export function getRefreshTokenFromRequest(request: Request): string | null {
  const header = request.headers.get('Cookie')
  if (!header) return null
  for (const part of header.split(';')) {
    const [rawName, ...rest] = part.trim().split('=')
    if (rawName === REFRESH_COOKIE) {
      const value = rest.join('=')
      return value ? decodeURIComponent(value) : null
    }
  }
  return null
}

export function buildRefreshSetCookie(
  refreshToken: string,
  maxAge = DEFAULT_MAX_AGE,
): string {
  const secure = process.env.COOKIE_SECURE === 'true'
  const parts = [
    `${REFRESH_COOKIE}=${encodeURIComponent(refreshToken)}`,
    'Path=/',
    `Max-Age=${maxAge}`,
    'HttpOnly',
    'SameSite=Lax',
  ]
  if (secure) parts.push('Secure')
  return parts.join('; ')
}

export function buildRefreshClearCookie(): string {
  const secure = process.env.COOKIE_SECURE === 'true'
  const parts = [
    `${REFRESH_COOKIE}=`,
    'Path=/',
    'Max-Age=0',
    'HttpOnly',
    'SameSite=Lax',
  ]
  if (secure) parts.push('Secure')
  return parts.join('; ')
}

async function authFetch(
  path: string,
  init?: RequestInit,
): Promise<Response> {
  return fetch(`${backendAuthBaseURL()}${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
  })
}

export async function backendLogin(
  email: string,
  password: string,
): Promise<TokenPair> {
  const res = await authFetch('/login', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Response(text || 'login failed', { status: res.status })
  }
  return (await res.json()) as TokenPair
}

export async function backendRefresh(refreshToken: string): Promise<TokenPair> {
  const res = await authFetch('/refresh', {
    method: 'POST',
    body: JSON.stringify({ refreshToken }),
  })
  if (!res.ok) {
    throw new Response('refresh failed', { status: res.status })
  }
  return (await res.json()) as TokenPair
}

export async function backendValidateSession(
  refreshToken: string,
): Promise<boolean> {
  const res = await authFetch('/session', {
    method: 'POST',
    body: JSON.stringify({ refreshToken }),
  })
  return res.ok
}

export async function backendLogout(refreshToken: string | null): Promise<void> {
  if (!refreshToken) return
  await authFetch('/logout', {
    method: 'POST',
    body: JSON.stringify({ refreshToken }),
  }).catch(() => undefined)
}

export async function probeSessionFromRequest(
  request: Request,
): Promise<boolean> {
  const rt = getRefreshTokenFromRequest(request)
  if (!rt) return false
  return backendValidateSession(rt)
}
