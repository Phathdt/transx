import { useEffect, type ReactNode } from 'react'
import { Link, NavLink, useNavigate } from 'react-router'
import { ArrowLeftRight, LogOut, Plus, Wallet } from 'lucide-react'
import { Button } from '#/components/ui/button'
import { InboxBell } from '#/components/inbox/inbox-bell'
import { useAuth } from '#/hooks/use-auth'
import { cn } from '#/lib/utils'

const NAV_ITEMS = [
  {
    to: '/app/transfers',
    label: 'Transfers',
    icon: ArrowLeftRight,
    end: true,
  },
  { to: '/app/transfers/new', label: 'New Transfer', icon: Plus, end: false },
  { to: '/app/accounts', label: 'Accounts', icon: Wallet, end: true },
] as const

/**
 * Authenticated app chrome: frosted top nav (Transfers, New Transfer, Accounts),
 * inbox bell, plus logout. Logout is intentionally separated from transfer
 * actions.
 */
export function AppShell({ children }: { children: ReactNode }) {
  const navigate = useNavigate()
  const { logout, status, isAuthenticated } = useAuth()

  async function handleLogout() {
    // useAuth.logout already navigates to /login after clearing session.
    await logout()
  }

  useEffect(() => {
    if (status === 'guest') {
      navigate('/login', { replace: true })
    }
  }, [status, navigate])

  // Wait for silent AT bootstrap (cookie RT → BFF /api/auth/refresh) before
  // mounting domain queries. Otherwise InboxBell / list pages fire without
  // Authorization and Traefik ForwardAuth returns 401 ("missing bearer token").
  if (status === 'loading' || status === 'guest') {
    return (
      <div className="flex min-h-dvh items-center justify-center text-sm text-muted-foreground">
        {status === 'guest' ? 'Redirecting to sign in…' : 'Loading session…'}
      </div>
    )
  }

  if (!isAuthenticated) {
    return null
  }

  return (
    <div className="flex min-h-dvh flex-col">
      <header className="app-header sticky top-0 z-40">
        <nav className="page-wrap flex h-16 items-center justify-between gap-4">
          <div className="flex items-center gap-2 sm:gap-7">
            <Link
              to="/app/transfers"
              className="flex items-center gap-2.5 no-underline"
            >
              <span className="brand-mark size-9">
                <ArrowLeftRight className="size-[18px]" strokeWidth={2.4} />
              </span>
              <span className="brand-wordmark display-title text-xl font-bold">
                transx
              </span>
            </Link>
            <div className="hidden items-center gap-1 sm:flex">
              {NAV_ITEMS.map(({ to, label, icon: Icon, end }) => (
                <NavLink
                  key={to}
                  to={to}
                  end={end}
                  className={({ isActive }) =>
                    cn(
                      'nav-link flex items-center gap-1.5 rounded-full px-3 py-1.5 text-sm font-medium',
                      isActive && 'is-active',
                    )
                  }
                >
                  <Icon className="size-4" />
                  {label}
                </NavLink>
              ))}
            </div>
          </div>
          <div className="flex items-center gap-1">
            <InboxBell />
            <Button variant="ghost" size="sm" onClick={handleLogout}>
              <LogOut className="size-4" />
              <span className="hidden sm:inline">Logout</span>
            </Button>
          </div>
        </nav>
        <div className="page-wrap flex items-center gap-1 pb-2 sm:hidden">
          {NAV_ITEMS.map(({ to, label, icon: Icon, end }) => (
            <NavLink
              key={to}
              to={to}
              end={end}
              className={({ isActive }) =>
                cn(
                  'nav-link flex flex-1 items-center justify-center gap-1.5 rounded-full px-2 py-1.5 text-xs font-medium',
                  isActive && 'is-active',
                )
              }
            >
              <Icon className="size-4" />
              {label}
            </NavLink>
          ))}
        </div>
      </header>
      <main className="page-wrap w-full flex-1 py-8 sm:py-10">{children}</main>
      <footer className="site-footer mt-auto">
        <div className="page-wrap flex items-center justify-between py-5 text-xs text-muted-foreground">
          <span className="display-title font-semibold text-[var(--sea-ink-soft)]">
            transx
          </span>
          <span>Simple wallet transfers</span>
        </div>
      </footer>
    </div>
  )
}
