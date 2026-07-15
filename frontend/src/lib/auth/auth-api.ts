import type { DtoLoginCommand, DtoLoginResponse } from '#/lib/api/generated/models'
import { clearSession, setSession } from '#/lib/auth/auth-session'
import { toApiError } from '#/lib/api/api-error'
import Axios from 'axios'

/**
 * Domain APIs (wallet/transfer/inbox) still hit Traefik with Bearer AT.
 * Auth bootstrap (login/refresh/logout) goes same-origin to the RR BFF so the
 * HttpOnly refresh cookie stays on the FE host.
 */
export function apiBaseURL(): string {
  return import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:4000/api/v1'
}

const bff = Axios.create({
  baseURL: '',
  withCredentials: true,
  headers: { 'Content-Type': 'application/json' },
})

type AuthJSON = {
  accessToken?: string
  tokenType?: string
  userId?: string
  userName?: string
}

function applySession(data: AuthJSON): DtoLoginResponse {
  setSession({
    accessToken: data.accessToken ?? '',
    tokenType: data.tokenType ?? 'Bearer',
    userId: data.userId ?? '',
    userName: data.userName ?? '',
  })
  return {
    accessToken: data.accessToken,
    tokenType: data.tokenType,
    userId: data.userId,
    userName: data.userName,
  }
}

/** Browser → RR BFF /api/auth/login → Go auth. Cookie set by RR. */
export async function loginRequest(
  command: DtoLoginCommand,
): Promise<DtoLoginResponse> {
  try {
    const { data } = await bff.post<AuthJSON>('/api/auth/login', command)
    return applySession(data)
  } catch (error) {
    throw toApiError(error)
  }
}

/** Client silent refresh via RR BFF (cookie included automatically). */
export async function refreshSession(): Promise<DtoLoginResponse> {
  try {
    const { data } = await bff.post<AuthJSON>('/api/auth/refresh')
    return applySession(data)
  } catch (error) {
    clearSession()
    throw toApiError(error)
  }
}

export async function logoutRequest(): Promise<void> {
  try {
    await bff.post('/api/auth/logout')
  } catch {
    // best-effort
  } finally {
    clearSession()
  }
}
