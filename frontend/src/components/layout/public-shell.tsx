import type { ReactNode } from 'react'

/**
 * Minimal centered chrome for public pages (login). No app navigation so the
 * login screen never inherits the authenticated shell.
 */
export function PublicShell({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center px-4 py-12">
      <div className="mb-8 text-center">
        <p className="island-kicker mb-1">transx</p>
        <h1 className="display-title text-2xl font-bold text-[var(--sea-ink)]">
          Simple Bank
        </h1>
      </div>
      {children}
    </div>
  )
}
