import { createFileRoute, Outlet, redirect } from '@tanstack/react-router'
import { AppShell } from '#/components/layout/app-shell'
import { authCheckQueryOptions } from '#/lib/auth/auth-query'
import { clearSession, getAccessToken } from '#/lib/auth/auth-session'

export const Route = createFileRoute('/app')({
  beforeLoad: async ({ context }) => {
    // No token at all -> straight to login, no /check call.
    if (!getAccessToken()) {
      throw redirect({ to: '/login' })
    }
    // Token exists -> confirm it is valid via /check before rendering the app.
    try {
      await context.queryClient.ensureQueryData(authCheckQueryOptions())
    } catch {
      clearSession()
      throw redirect({ to: '/login' })
    }
  },
  component: AppLayout,
})

function AppLayout() {
  return (
    <AppShell>
      <Outlet />
    </AppShell>
  )
}
