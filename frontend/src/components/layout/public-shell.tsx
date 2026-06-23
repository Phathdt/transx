import type { ReactNode } from 'react'
import { ArrowLeftRight } from 'lucide-react'

/**
 * Minimal centered chrome for public pages (login). No app navigation so the
 * login screen never inherits the authenticated shell.
 */
export function PublicShell({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-dvh flex-col items-center justify-center px-4 py-12">
      <div className="rise-in mb-8 flex flex-col items-center text-center">
        <span className="brand-mark mb-4 size-14">
          <ArrowLeftRight className="size-7" strokeWidth={2.4} />
        </span>
        <p className="island-kicker mb-1">transx</p>
        <h1 className="brand-wordmark display-title text-3xl font-bold">
          Simple Bank
        </h1>
        <p className="mt-2 max-w-xs text-sm text-muted-foreground">
          Move money between wallets, fast and predictable.
        </p>
      </div>
      <div className="rise-in w-full max-w-sm">{children}</div>
    </div>
  )
}
