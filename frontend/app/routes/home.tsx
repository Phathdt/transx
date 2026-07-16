import { redirect } from 'react-router'
import type { Route } from './+types/home'
import { probeSessionFromRequest } from '../lib/auth.server'

export async function loader({ request }: Route.LoaderArgs) {
  const ok = await probeSessionFromRequest(request)
  throw redirect(ok ? '/app/transfers' : '/login')
}

export default function Home() {
  return null
}
