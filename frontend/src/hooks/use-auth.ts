import { useCallback } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useRouter } from '@tanstack/react-router'
import { authCheckQueryOptions, loginRequest } from '#/lib/auth/auth-query'
import { clearSession, getSession, setSession } from '#/lib/auth/auth-session'
import type { ApiError } from '#/lib/api/api-error'
import type { DtoLoginCommand } from '#/lib/api/generated/models'

export type AuthStatus = 'loading' | 'authenticated' | 'guest'

/**
 * Domain auth hook. Components consume this instead of generated hooks directly,
 * keeping token handling in one place. Token existence only enables `/check`;
 * `/check` success is the real authenticated signal.
 */
export function useAuth() {
  const queryClient = useQueryClient()
  const router = useRouter()
  const session = getSession()
  const hasToken = Boolean(session?.accessToken)

  const checkQuery = useQuery({
    ...authCheckQueryOptions(),
    enabled: hasToken,
  })

  const loginMutation = useMutation<
    Awaited<ReturnType<typeof loginRequest>>,
    ApiError,
    DtoLoginCommand
  >({
    mutationFn: loginRequest,
    onSuccess: (data) => {
      setSession({
        accessToken: data.accessToken ?? '',
        tokenType: data.tokenType ?? 'Bearer',
        userId: data.userId ?? '',
      })
    },
  })

  const logout = useCallback(async () => {
    clearSession()
    queryClient.clear()
    await router.invalidate()
  }, [queryClient, router])

  let status: AuthStatus = 'guest'
  if (hasToken) {
    if (checkQuery.isLoading) status = 'loading'
    else if (checkQuery.isSuccess) status = 'authenticated'
    else status = 'guest'
  }

  return {
    status,
    session,
    userId: session?.userId,
    isAuthenticated: status === 'authenticated',
    login: loginMutation,
    logout,
  }
}
