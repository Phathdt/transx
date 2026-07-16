import { Outlet, redirect, useLoaderData } from 'react-router'
import type { Route } from './+types/app-layout'
import { AppShell } from '#/components/layout/app-shell'
import { getInboxUnreadCount } from '../lib/api/generated/inbox/inbox'
import { probeSessionFromRequest } from '../lib/auth.server'
import { withServerBearer } from '../lib/server-domain.server'

function isRedirectResponse(err: unknown): err is Response {
  return err instanceof Response && err.status >= 300 && err.status < 400
}

/**
 * Auth-gate + SSR unread badge seed.
 * RT cookie → Go POST /session (no rotation). Unread via AT_ssr for first HTML paint;
 * InboxBell hydrates React Query and polls with AT_browser after silent renew.
 */
export async function loader({ request }: Route.LoaderArgs) {
  const ok = await probeSessionFromRequest(request)
  if (!ok) throw redirect('/login')

  let unreadCount = 0
  try {
    const res = await withServerBearer(request, (init) =>
      getInboxUnreadCount(init),
    )
    unreadCount = res.count ?? 0
  } catch (err) {
    // Session loss → login. Inbox outage must not blank the whole app shell.
    if (isRedirectResponse(err)) throw err
    if (err instanceof Response && err.status === 401) throw redirect('/login')
  }

  return { unreadCount }
}

/** Client SPA navigations keep layout data; RQ owns live unread after hydrate. */
export function shouldRevalidate() {
  return false
}

export default function AppLayout() {
  const { unreadCount } = useLoaderData<typeof loader>()
  return (
    <AppShell initialUnreadCount={unreadCount}>
      <Outlet />
    </AppShell>
  )
}
