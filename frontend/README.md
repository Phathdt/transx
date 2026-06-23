# transx — Simple Bank Frontend

Web UI for the transx wallet transfer system: log in with a seeded account,
view your transfer history, create internal/external transfers, and follow a
transfer through to settlement.

Built with Vite + React 19 (client-side SPA), TanStack Router + Query, Tailwind
v4, shadcn/ui, and an Orval-generated API client (React Query hooks + Zod
schemas).

## Prerequisites

The frontend talks to the transx backend through the Traefik gateway. Start the
backend stack first (from the repo root):

```bash
docker compose up -d          # Postgres, Redpanda, Traefik, services
cd backend && make migrate && make seed
```

The gateway listens on `http://localhost:4000` and all API routes live under
`/api/v1`. See the root [`README.md`](../README.md) for the full backend setup.

## Getting Started

```bash
yarn install
cp .env.example .env          # set VITE_API_PROXY_TARGET if not localhost:4000
yarn dev                      # http://localhost:3000
```

The dev server proxies `/api/v1/*` to `VITE_API_PROXY_TARGET` (default
`http://localhost:4000`), so the browser calls the API same-origin and the
bearer token stays first-party.

### Dev login

Seeded accounts (from `../backend/cli/seed.go`), password `password123`:

| Email                      | Accounts                              |
| -------------------------- | ------------------------------------- |
| `alice@transx.dev`         | `alice-main` (USD), `alice-vnd` (VND) |
| `bob@transx.dev`           | `bob-main` (USD)                      |
| `carol@…` `dave@…` `eve@…` | seeded users, no accounts             |

## API Client Generation

The typed API client is generated from the backend OpenAPI spec
(`../backend/openapi.yaml`) by [Orval](https://orval.dev/):

```bash
yarn generate:api
```

This writes React Query hooks + Zod schemas to `src/lib/api/generated/` (do not
edit by hand). Regenerate whenever the backend spec changes. Config lives in
`orval.config.ts`; the Axios mutator (`src/lib/api/http-mutator.ts`) injects the
`Authorization: Bearer` header and normalizes errors. `X-User-Id` is injected by
the gateway's ForwardAuth, never by the browser.

## Routes

| Route                        | Purpose                                  |
| ---------------------------- | ---------------------------------------- |
| `/login`                     | Public login page                        |
| `/app/transfers`             | Transfer history (paginated)             |
| `/app/transfers/new`         | Create a transfer (lookup + idempotency) |
| `/app/transfers/$transferId` | Transfer detail with status polling      |

`/app` is a protected layout: it redirects to `/login` without a valid token
(verified via `GET /check`). Regenerate the route tree after adding routes:

```bash
yarn generate-routes
```

## Scripts

```bash
yarn dev              # dev server on :3000
yarn build            # production build (static SPA in dist/)
yarn test             # vitest
yarn run lint         # eslint
yarn run check        # prettier --check
yarn run format       # prettier --write && eslint --fix
yarn generate:api     # regenerate API client from backend OpenAPI
yarn generate-routes  # regenerate the router tree
```

## Styling

Tailwind CSS v4 with shadcn/ui (new-york style). Add primitives with:

```bash
npx shadcn@latest add <component>
```
