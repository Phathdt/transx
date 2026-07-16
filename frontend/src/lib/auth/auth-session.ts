/**
 * In-memory auth session. Access tokens never touch localStorage/sessionStorage
 * (XSS posture for hybrid auth). Refresh token lives in HttpOnly cookie only.
 */

export interface AuthSession {
  accessToken: string
  tokenType: string
  userId: string
  userName: string
}

let session: AuthSession | null = null

export function getSession(): AuthSession | null {
  return session
}

export function getAccessToken(): string | null {
  return session?.accessToken ?? null
}

export function setSession(next: AuthSession): void {
  session = {
    accessToken: next.accessToken,
    tokenType: next.tokenType || 'Bearer',
    userId: next.userId,
    userName: next.userName,
  }
}

export function clearSession(): void {
  session = null
}
