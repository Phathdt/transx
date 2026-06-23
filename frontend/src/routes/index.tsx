import { createFileRoute, redirect } from '@tanstack/react-router'
import { getAccessToken } from '#/lib/auth/auth-session'

export const Route = createFileRoute('/')({
  beforeLoad: () => {
    // Single entry point: route to the app if signed in, else to login.
    throw redirect({ to: getAccessToken() ? '/app/transfers' : '/login' })
  },
})
