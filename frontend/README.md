# transx — Simple Bank Frontend

Web UI for the transx wallet transfer system.

Built with **React Router v7 framework mode** (SSR) + React 19, TanStack Query,
Tailwind v4, shadcn/ui, Orval API client.

## Auth model (pivot A — Auth BFF)

```text
Browser ──same-origin──► RR Node (/api/auth/*)
                            │ HttpOnly refresh cookie
                            ▼
                         Go Auth (JSON AT + RT, Redis)
Browser ──Bearer AT────► Traefik → wallet/transfer/inbox
```

- **Access token:** memory only
- **Refresh token:** HttpOnly cookie set by **RR**, not Go
- **Login / refresh / logout:** browser → RR BFF → Go
- **Domain APIs:** browser → Traefik with Bearer AT
- **SSR loaders:** auth-gate via cookie → Go `POST /session` (no rotation)

## Prerequisites

```bash
docker compose up -d
cd backend && make migrate && make seed
```

Gateway: `http://localhost:4000`. Redis required for refresh sessions.

## Getting Started

```bash
yarn install
cp .env.example .env
yarn dev   # http://localhost:3000
```

| Env | Purpose |
|---|---|
| `VITE_API_BASE_URL` | Domain API base (Traefik), default `http://localhost:4000/api/v1` |
| `AUTH_API_BASE_URL` | Server-side RR → Go auth (defaults to same) |
| `COOKIE_SECURE` | `true` behind HTTPS |

### Dev login

Password `password123`: `alice@transx.dev`, `bob@transx.dev`, …

## Routes

| Path | Purpose |
|---|---|
| `/login` | Public login |
| `/api/auth/login\|refresh\|logout` | RR auth BFF |
| `/app/transfers` … | Protected app (layout auth-gate) |
| `/app/accounts` … | Accounts |

## Scripts

```bash
yarn dev          # react-router dev :3000
yarn build        # SSR + client → build/
yarn start        # serve production SSR
yarn test
yarn generate:api # Orval from backend OpenAPI
```
