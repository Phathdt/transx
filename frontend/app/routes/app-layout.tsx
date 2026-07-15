import { Outlet, redirect } from 'react-router'
import type { Route } from './+types/app-layout'
import { AppShell } from '#/components/layout/app-shell'
import { probeSessionFromRequest } from '../lib/auth.server'

/**
 * Auth-gate only: RT cookie → Go POST /session (no rotation, no AT in loader data).
 */
export async function loader({ request }: Route.LoaderArgs) {
  const ok = await probeSessionFromRequest(request)
  if (!ok) throw redirect('/login')
  return null
}

export default function AppLayout() {
  return (
    <AppShell>
      <Outlet />
    </AppShell>
  )
}
