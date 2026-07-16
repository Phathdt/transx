/**
 * React Router BFF auth helpers.
 * Owns the HttpOnly refresh-token cookie. Go auth only speaks JSON tokens.
 * Silent browser AT renew uses Go POST /session/access (no RT rotate).
 *
 * Go HTTP calls go through Orval server client (app/lib/api/generated/auth).
 * Cookie + Redis dual-AT stay here — Orval does not own those.
 */

import { parseCookie } from 'cookie'
import {
  login as goLogin,
  logout as goLogout,
  refresh as goRefresh,
  session as goSession,
  sessionAccess as goSessionAccess,
} from './api/generated/auth/auth'
import type {
  DtoLoginResponse,
  DtoServerAccessResponse,
} from './api/generated/models'
import {
  deleteServerAccessToken,
  getServerAccessTokenBySession,
  parseSessionId,
  putServerAccessToken,
} from './rr-at-cache.server'
import { authApiBaseURL } from './api/server-http-mutator'

const REFRESH_COOKIE = 'refresh_token'
const DEFAULT_MAX_AGE = 60 * 60 * 24 // 1d (match auth.refresh_ttl)

/** Login/refresh pair from Go (AT + RT). */
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

export { authApiBaseURL as backendAuthBaseURL }

function requireTokenPair(data: DtoLoginResponse, failMessage: string): TokenPair {
  if (!data.accessToken || !data.refreshToken) {
    throw new Response(failMessage, { status: 502 })
  }
  return {
    accessToken: data.accessToken,
    refreshToken: data.refreshToken,
    tokenType: data.tokenType,
    userId: data.userId,
    userName: data.userName,
  }
}

function requireAccessToken(
  data: DtoServerAccessResponse,
  failMessage: string,
): AccessTokenOnly {
  if (!data.accessToken) {
    throw new Response(failMessage, { status: 502 })
  }
  return {
    accessToken: data.accessToken,
    tokenType: data.tokenType,
    userId: data.userId,
    userName: data.userName,
  }
}

export function getRefreshTokenFromRequest(request: Request): string | null {
  const header = request.headers.get('Cookie')
  if (!header) return null
  const cookies = parseCookie(header)
  const value = cookies[REFRESH_COOKIE]
  return value ? value : null
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

export async function backendLogin(
  email: string,
  password: string,
): Promise<TokenPair> {
  const data = await goLogin({ email, password })
  return requireTokenPair(data, 'login failed')
}

/** Optional forced RT rotation; silent renew must use backendSessionAccess. */
export async function backendRefresh(refreshToken: string): Promise<TokenPair> {
  const data = await goRefresh({ refreshToken })
  return requireTokenPair(data, 'refresh failed')
}

/** Mint a new access token only; RT session unchanged. */
export async function backendSessionAccess(
  refreshToken: string,
): Promise<AccessTokenOnly> {
  const data = await goSessionAccess({ refreshToken })
  return requireAccessToken(data, 'session access failed')
}

export async function backendValidateSession(
  refreshToken: string,
): Promise<boolean> {
  try {
    await goSession({ refreshToken })
    return true
  } catch {
    return false
  }
}

export async function backendLogout(refreshToken: string | null): Promise<void> {
  if (!refreshToken) return
  await goLogout({ refreshToken }).catch(() => undefined)
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
