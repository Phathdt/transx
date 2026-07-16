import { useCallback, useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router'
import { authCheckQueryOptions } from '#/lib/auth/auth-query'
import {
  loginRequest,
  logoutRequest,
  refreshSession,
} from '#/lib/auth/auth-api'
import {
  clearSession,
  getAccessToken,
  getSession,
} from '#/lib/auth/auth-session'
import type { ApiError } from '#/lib/api/api-error'
import type { DtoLoginCommand } from '#/lib/api/generated/models'

export type AuthStatus = 'loading' | 'authenticated' | 'guest'

/**
 * Domain auth hook. Memory AT + HttpOnly refresh cookie.
 * On mount, if memory is empty, silently refresh from cookie once.
 */
export function useAuth() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const [bootstrapped, setBootstrapped] = useState(() => Boolean(getAccessToken()))

  useEffect(() => {
    if (getAccessToken()) {
      setBootstrapped(true)
      return
    }
    let cancelled = false
    refreshSession()
      .catch(() => {
        clearSession()
      })
      .finally(() => {
        if (!cancelled) setBootstrapped(true)
      })
    return () => {
      cancelled = true
    }
  }, [])

  const session = getSession()
  const hasToken = Boolean(session?.accessToken)

  const checkQuery = useQuery({
    ...authCheckQueryOptions(),
    enabled: bootstrapped && hasToken,
  })

  const loginMutation = useMutation<
    Awaited<ReturnType<typeof loginRequest>>,
    ApiError,
    DtoLoginCommand
  >({
    mutationFn: loginRequest,
  })

  const logout = useCallback(async () => {
    await logoutRequest()
    queryClient.clear()
    navigate('/login')
  }, [queryClient, navigate])

  let status: AuthStatus = 'guest'
  if (!bootstrapped) {
    status = 'loading'
  } else if (hasToken) {
    if (checkQuery.isLoading) status = 'loading'
    else if (checkQuery.isSuccess) status = 'authenticated'
    else status = 'guest'
  }

  return {
    status,
    session,
    userId: session?.userId,
    userName: session?.userName,
    isAuthenticated: status === 'authenticated',
    login: loginMutation,
    logout,
  }
}
