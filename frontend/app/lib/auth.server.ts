/**
 * React Router BFF auth helpers.
 * Owns the HttpOnly refresh-token cookie. Go auth only speaks JSON tokens.
 * Silent browser AT renew uses Go POST /session/access (no RT rotate).
 */

import {
  deleteServerAccessToken,
  getServerAccessTokenBySession,
  parseSessionId,
  putServerAccessToken,
} from './rr-at-cache.server'

const REFRESH_COOKIE = 'refresh_token'
const DEFAULT_MAX_AGE = 60 * 60 * 24 * 30 // 30d

export type TokenPair = {
  accessToken: string
  refreshToken: string
  tokenType?: string
  userId?: string
  userName?: string
}

/** AT mint response from Go /session/access (no RT). Includes user for cold bootstrap. */
export type AccessTokenOnly = {
  accessToken: string
  tokenType?: string
  userId?: string
  userName?: string
}

export function backendAuthBaseURL(): string {
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

function buildRefreshCookie(value: string, maxAge: number): string {
  const parts = [
    `${REFRESH_COOKIE}=${value}`,
    'Path=/',
    `Max-Age=${maxAge}`,
    'HttpOnly',
    'SameSite=Lax',
  ]
  if (process.env.COOKIE_SECURE === 'true') parts.push('Secure')
  return parts.join('; ')
}

export function buildRefreshSetCookie(
  refreshToken: string,
  maxAge = DEFAULT_MAX_AGE,
): string {
  return buildRefreshCookie(encodeURIComponent(refreshToken), maxAge)
}

export function buildRefreshClearCookie(): string {
  return buildRefreshCookie('', 0)
}

async function authFetch(path: string, init?: RequestInit): Promise<Response> {
  return fetch(`${backendAuthBaseURL()}${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
  })
}

async function authJSON<T>(
  path: string,
  body: unknown,
  failMessage: string,
): Promise<T> {
  const res = await authFetch(path, {
    method: 'POST',
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const text = path === '/login' ? await res.text() : ''
    throw new Response(text || failMessage, { status: res.status })
  }
  return (await res.json()) as T
}

export async function backendLogin(
  email: string,
  password: string,
): Promise<TokenPair> {
  return authJSON<TokenPair>('/login', { email, password }, 'login failed')
}

/** Optional forced RT rotation; silent renew must use backendSessionAccess. */
export async function backendRefresh(refreshToken: string): Promise<TokenPair> {
  return authJSON<TokenPair>('/refresh', { refreshToken }, 'refresh failed')
}

/** Mint a new access token only; RT session unchanged. */
export async function backendSessionAccess(
  refreshToken: string,
): Promise<AccessTokenOnly> {
  return authJSON<AccessTokenOnly>(
    '/session/access',
    { refreshToken },
    'session access failed',
  )
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

/**
 * Server-only SSR AT: cache hit or mint via /session/access and cache.
 * Does not touch the browser cookie or rotate RT.
 */
export async function getServerAccessToken(
  request: Request,
): Promise<string | null> {
  const rt = getRefreshTokenFromRequest(request)
  if (!rt) return null
  const sid = parseSessionId(rt)
  if (!sid) return null

  const cached = await getServerAccessTokenBySession(sid)
  if (cached) return cached

  const minted = await backendSessionAccess(rt)
  await putServerAccessToken(sid, minted.accessToken)
  return minted.accessToken
}

export async function clearServerAccessTokenForRefresh(
  refreshToken: string | null,
): Promise<void> {
  if (!refreshToken) return
  const sid = parseSessionId(refreshToken)
  if (!sid) return
  await deleteServerAccessToken(sid)
}

export { parseSessionId, putServerAccessToken, deleteServerAccessToken }
