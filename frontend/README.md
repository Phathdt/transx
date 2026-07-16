# transx — Simple Bank Frontend

Web UI for the transx wallet transfer system.

Built with **React Router v7 framework mode** (SSR) + React 19, TanStack Query,
Tailwind v4, shadcn/ui, Orval API client.

## Auth model (dual access tokens)

```text
Browser ──same-origin──► RR Node (/api/auth/*)
                            │ HttpOnly refresh cookie
                            │ RR Redis rr:at:{sessionID} (AT_ssr)
                            ▼
                         Go Auth (JSON)
                            │ Go Redis auth:rt:{sessionID}
Browser ──Bearer AT────► Traefik → wallet/transfer/inbox
```

| Token | Where | How obtained |
|---|---|---|
| **AT_browser** | Memory only | Login JSON; silent renew via BFF |
| **AT_ssr** | RR Redis `rr:at:{sid}` | Login hop + cache miss → Go `/session/access` |
| **RT** | HttpOnly cookie (RR host) | Login; stable across silent AT renew |

- **Login:** Go `/login` → AT_browser+RT; Go `/session/access` → AT_ssr; cache; cookie RT
- **Silent renew** (`POST /api/auth/refresh`): cookie RT → Go **`/session/access`** → AT only; **no Set-Cookie**; does **not** call Go `/refresh`
- **Logout:** revoke this RT + DEL this `rr:at` key + clear cookie
- **Multi-device:** concurrent sessions; logout A keeps B
- **Domain APIs:** browser → Traefik with Bearer AT
- **SSR loaders:** auth-gate via cookie → Go `POST /session` (no rotation)

## Prerequisites

```bash
docker compose up -d   # includes redis (Go) + redis-rr (RR SSR AT)
cd backend && make migrate && make seed
```

Gateway: `http://localhost:4000`. Two Redis instances:

| Service | Host port (default) | Keys |
|---|---|---|
| `redis` | 16379 | Go `auth:rt:*` |
| `redis-rr` | 16380 | RR `rr:at:*` |

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
| `RR_REDIS_URL` | SSR AT cache Redis, default `redis://localhost:16380` |
| `RR_AT_TTL_SECONDS` | Cache TTL, default `900` (≈ JWT TTL) |
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
