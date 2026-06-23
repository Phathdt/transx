import type { ReactNode } from 'react'
import { Link, useRouter } from '@tanstack/react-router'
import { LogOut } from 'lucide-react'
import { Button } from '#/components/ui/button'
import { useAuth } from '#/hooks/use-auth'

/**
 * Authenticated app chrome: top nav (Transfers, New Transfer) plus logout.
 * Logout is intentionally separated from transfer actions.
 */
export function AppShell({ children }: { children: ReactNode }) {
  const { logout } = useAuth()
  const router = useRouter()

  async function handleLogout() {
    await logout()
    await router.navigate({ to: '/login' })
  }

  return (
    <div className="min-h-screen">
      <header className="site-footer sticky top-0 z-10 border-b">
        <nav className="page-wrap flex items-center justify-between py-3">
          <div className="flex items-center gap-6">
            <Link
              to="/app/transfers"
              className="display-title text-lg font-bold no-underline"
            >
              transx
            </Link>
            <div className="flex items-center gap-4 text-sm">
              <Link
                to="/app/transfers"
                className="nav-link"
                activeProps={{ className: 'nav-link is-active' }}
                activeOptions={{ exact: true }}
              >
                Transfers
              </Link>
              <Link
                to="/app/transfers/new"
                className="nav-link"
                activeProps={{ className: 'nav-link is-active' }}
              >
                New Transfer
              </Link>
              <Link
                to="/app/accounts"
                className="nav-link"
                activeProps={{ className: 'nav-link is-active' }}
                activeOptions={{ exact: true }}
              >
                Accounts
              </Link>
            </div>
          </div>
          <Button variant="ghost" size="sm" onClick={handleLogout}>
            <LogOut className="size-4" />
            Logout
          </Button>
        </nav>
      </header>
      <main className="page-wrap py-8">{children}</main>
    </div>
  )
}
