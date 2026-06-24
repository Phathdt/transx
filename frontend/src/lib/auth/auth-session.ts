/**
 * Single source of truth for the local auth session. Token lives in
 * localStorage for the local-first dev phase (XSS trade-off noted in the plan).
 * All access is guarded for SSR (TanStack Start renders on the server).
 */
const STORAGE_KEY = 'transx.auth.session'

export interface AuthSession {
  accessToken: string
  tokenType: string
  userId: string
  userName: string
}

function hasWindow(): boolean {
  return typeof window !== 'undefined'
}

export function getSession(): AuthSession | null {
  if (!hasWindow()) return null
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as AuthSession
    if (!parsed.accessToken) return null
    return parsed
  } catch {
    return null
  }
}

export function getAccessToken(): string | null {
  return getSession()?.accessToken ?? null
}

export function setSession(session: AuthSession): void {
  if (!hasWindow()) return
  window.localStorage.setItem(STORAGE_KEY, JSON.stringify(session))
}

export function clearSession(): void {
  if (!hasWindow()) return
  window.localStorage.removeItem(STORAGE_KEY)
}
