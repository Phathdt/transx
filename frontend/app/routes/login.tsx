import { redirect } from 'react-router'
import type { Route } from './+types/login'
import { PublicShell } from '#/components/layout/public-shell'
import { LoginForm } from '#/components/auth/login-form'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '#/components/ui/card'
import { probeSessionFromRequest } from '../lib/auth.server'

export async function loader({ request }: Route.LoaderArgs) {
  const ok = await probeSessionFromRequest(request)
  if (ok) throw redirect('/app/transfers')
  return null
}

export default function LoginPage() {
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
