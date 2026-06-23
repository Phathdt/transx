import { createFileRoute, redirect } from '@tanstack/react-router'
import { PublicShell } from '#/components/layout/public-shell'
import { LoginForm } from '#/components/auth/login-form'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '#/components/ui/card'
import { getAccessToken } from '#/lib/auth/auth-session'

export const Route = createFileRoute('/login')({
  beforeLoad: () => {
    // Already-authenticated users skip the login screen.
    if (getAccessToken()) {
      throw redirect({ to: '/app/transfers' })
    }
  },
  component: LoginPage,
})

function LoginPage() {
  return (
    <PublicShell>
      <Card className="glass-card border-0 shadow-none">
        <CardHeader>
          <CardTitle>Sign in</CardTitle>
          <CardDescription>
            Use a seeded dev account to access your transfers.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <LoginForm />
          <p className="mt-4 text-xs text-muted-foreground">
            Dev seed: <code>alice@transx.dev</code> / <code>password123</code>
          </p>
        </CardContent>
      </Card>
    </PublicShell>
  )
}
