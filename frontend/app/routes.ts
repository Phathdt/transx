import {
  type RouteConfig,
  index,
  layout,
  route,
} from '@react-router/dev/routes'

export default [
  index('routes/home.tsx'),
  route('login', 'routes/login.tsx'),
  // Auth BFF (Browser → RR Node → Go). Cookie RT owned here.
  route('api/auth/login', 'routes/api.auth.login.ts'),
  route('api/auth/refresh', 'routes/api.auth.refresh.ts'),
  route('api/auth/logout', 'routes/api.auth.logout.ts'),
  layout('routes/app-layout.tsx', [
    route('app/transfers', 'routes/app.transfers.tsx'),
    route('app/transfers/new', 'routes/app.transfers.new.tsx'),
    route('app/transfers/:transferId', 'routes/app.transfers.$transferId.tsx'),
    route('app/accounts', 'routes/app.accounts.tsx'),
    route('app/accounts/:accountRef', 'routes/app.accounts.$accountRef.tsx'),
  ]),
] satisfies RouteConfig
