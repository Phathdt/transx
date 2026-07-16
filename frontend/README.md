# transx — Simple Bank Frontend

Web UI for the transx wallet transfer system.

Built with **React Router v8 framework mode** (SSR) + React 19, TanStack Query,
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

| Token          | Where                     | How obtained                                  |
| -------------- | ------------------------- | --------------------------------------------- |
| **AT_browser** | Memory only               | Login JSON; silent renew via BFF              |
| **AT_ssr**     | RR Redis `rr:at:{sid}`    | Login hop + cache miss → Go `/session/access` |
| **RT**         | HttpOnly cookie (RR host) | Login; stable across silent AT renew          |

- **Login:** Go `/login` → AT_browser+RT; Go `/session/access` → AT_ssr; cache; cookie RT
- **Silent renew** (`POST /api/auth/refresh`): cookie RT → Go **`/session/access`** → AT only; **no Set-Cookie**; does **not** call Go `/refresh`
- **Logout:** revoke this RT + DEL this `rr:at` key + clear cookie
- **Multi-device:** concurrent sessions; logout A keeps B
- **Domain APIs (browser):** browser → Traefik with Bearer AT_browser
- **SSR loaders:** auth-gate via cookie → Go `POST /session`; domain pages (e.g. `/app/transfers`, `/app/accounts`) fetch with AT_ssr on Node and render HTML; layout seeds inbox unread badge (then React Query polls with AT_browser)

## Prerequisites

```bash
docker compose up -d   # includes redis (Go) + redis-rr (RR SSR AT) + frontend
cd backend && make migrate && make seed
```

Gateway: `http://localhost:4000`. UI: `http://localhost:3000`. Two Redis instances:

| Service    | Host port (default) | Keys           |
| ---------- | ------------------- | -------------- |
| `redis`    | 16379               | Go `auth:rt:*` |
| `redis-rr` | 16380               | RR `rr:at:*`   |

## Getting Started

### Local (pnpm)

```bash
pnpm install
cp .env.example .env
pnpm dev   # http://localhost:3000
```

### Docker Compose

```bash
# from repo root — builds frontend/Dockerfile and wires redis-rr + Traefik auth
docker compose up -d --build frontend
# UI: http://localhost:3000
```

| Env                 | Purpose                                                                                                        |
| ------------------- | -------------------------------------------------------------------------------------------------------------- |
| `VITE_API_BASE_URL` | Domain API base (browser → Traefik). **Build-time** for Docker (`ARG`); default `http://localhost:4000/api/v1` |
| `AUTH_API_BASE_URL` | Server-side RR → Go auth. Compose default `http://traefik/api/v1`                                              |
| `RR_REDIS_URL`      | SSR AT cache Redis. Local: `redis://localhost:16380`; Compose: `redis://redis-rr:6379`                         |
| `RR_AT_TTL_SECONDS` | Cache TTL, default `900` (≈ JWT TTL)                                                                           |
| `COOKIE_SECURE`     | `true` behind HTTPS                                                                                            |
| `FRONTEND_PORT`     | Host port for compose frontend, default `3000`                                                                 |

### Dev login

Password `password123`: `alice@transx.dev`, `bob@transx.dev`, …

## Routes

| Path                               | Purpose                          |
| ---------------------------------- | -------------------------------- |
| `/login`                           | Public login                     |
| `/api/auth/login\|refresh\|logout` | RR auth BFF                      |
| `/app/transfers`                   | Transfer list (SSR loader + HTML) |
| `/app/transfers/new\|:id` …        | Protected app (layout auth-gate) |
| `/app/accounts`                    | Account list (SSR loader + HTML) |
| `/app/accounts/:accountRef`        | Account detail (client fetch)    |

## Scripts

```bash
pnpm dev          # react-router dev :3000
pnpm build        # SSR + client → build/
pnpm start        # serve production SSR
pnpm test
pnpm generate:api # Orval from backend OpenAPI
```
