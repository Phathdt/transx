# PRD: Hybrid Authentication Architecture for React Router v7 + Go Backend

## Status

**Implemented (pivot A + dual AT)** — Auth BFF on React Router Node; Go auth is JSON tokens only. Browser AT and SSR AT are independent strings; silent renew mints AT only (no RT rotate).

### Implemented decisions

| Item | Choice |
|---|---|
| FE runtime | React Router v7 framework mode (`app/`, SSR) |
| Access token (browser) | Memory only; never web storage |
| Access token (SSR) | RR Redis `rr:at:{sessionID}` (dedicated `redis-rr`) |
| Refresh token | Redis-backed opaque session on **Go**; **HttpOnly cookie on RR host** |
| Topology | `Browser → RR Node (cookie RT) → Go Auth (JSON)` |
| Domain APIs | Browser → Traefik with **Bearer AT** (not full BFF) |
| Cookie SameSite | `Lax` on FE origin (same-site auth BFF routes) |
| SSR loaders | Auth-gate: cookie → Go `POST /session` (no rotation); SSR AT via cache/`/session/access` |
| Client AT renew | Silent `POST /api/auth/refresh` (RR BFF) → Go **`POST /session/access`** (AT only, cookie RT unchanged) |
| Go `/refresh` | Optional forced RT rotation; **not** silent browser path |
| Multi-device | Concurrent sessions; logout revokes **this** RT only |
| AT TTL / RT TTL | 15m / 1d (config); RR cache TTL ≈ JWT TTL |
| Out of scope | Full API BFF, domain SSR loaders, dual `aud`, logout-all |

Go public auth (no ForwardAuth): `/login`, `/session/access`, `/refresh`, `/logout`, `/session` — **JSON body**, no `Set-Cookie`.  
RR BFF routes: `/api/auth/login|refresh|logout` own the cookie.

Code anchors:

| Layer | Location |
|---|---|
| Go Access (AT only) | `backend/.../auth_service.go` `Access`; `POST /session/access` |
| Go Refresh (RT rotate) | `Refresh`; `POST /refresh` — optional |
| Go RT store | Redis key `auth:rt:{sessionID}` |
| RR BFF silent renew | `frontend/app/routes/api.auth.refresh.ts` → `backendSessionAccess` |
| RR SSR AT cache | `frontend/app/lib/rr-at-cache.server.ts` → `rr:at:{sessionID}` on `redis-rr` |

---

# 1. Overview

Authentication for:

- React Router v7 framework mode (SSR + Auth BFF)
- Go auth service (JSON AT/RT only; ForwardAuth `/check` for domain APIs)
- Dual independent access tokens (browser memory vs SSR Redis)
- Opaque refresh session in Go Redis; HttpOnly cookie only on the RR host

Goals:

- Browser calls domain APIs directly with Bearer AT (Traefik ForwardAuth)
- SSR loaders auth-gate with cookie RT without rotating it
- Silent browser AT renew without RT rotation or cookie rewrite
- No access token in LocalStorage / SessionStorage

---

# 2. High-Level Architecture

```text
Browser
   │ same-origin /api/auth/*
   │ HttpOnly RT cookie (RR host only)
   ▼
React Router Node (Auth BFF)
   │ Redis redis-rr: rr:at:{sessionID}  → AT_ssr
   │ JSON POST to Go (refreshToken body)
   ▼
Go Auth
   │ Redis redis: auth:rt:{sessionID}   → RT session hash
   │ issues JWT AT (no Set-Cookie)

Browser ──Bearer AT──► Traefik ──ForwardAuth /check──► domain APIs
```

- **RT** is the only shared long-lived credential (cookie on RR; opaque string to Go).
- **AT_browser** and **AT_ssr** are independent JWT strings (same claims shape; not shared).

---

# 3. Components

## Browser

| Stores | Responsibility |
|---|---|
| AT_browser (memory) | Domain API `Authorization: Bearer` |
| RT (HttpOnly cookie, RR host) | Sent only to RR `/api/auth/*` |

Never persists AT in web storage. Silent renew via same-origin `POST /api/auth/refresh`.

## React Router Node (Auth BFF)

| Stores | Responsibility |
|---|---|
| HttpOnly RT cookie | Set/clear on login/logout |
| `rr:at:{sid}` on `redis-rr` | SSR AT cache (TTL ≈ JWT TTL) |

Routes: `/api/auth/login`, `/api/auth/refresh`, `/api/auth/logout`.  
Does **not** proxy domain wallet/transfer/inbox APIs (out of scope full BFF).

## Go Auth

| Endpoint | Behavior |
|---|---|
| `POST /login` | Credentials → JSON `accessToken` + `refreshToken` (no cookie) |
| `POST /session/access` | Valid RT → new **AT only** (no RT rotate) |
| `POST /refresh` | Valid RT → new AT+RT (rotates; optional/forced) |
| `POST /session` | Valid RT → 204 (no rotate; auth-gate probe) |
| `POST /logout` | Revoke this RT session (idempotent) |
| `GET /check` | ForwardAuth: Bearer AT → `X-User-ID` |

RT material lives in Go Redis under `auth:rt:{sessionID}`.

---

# 4. Flows (as implemented)

## Login

```text
Browser → RR POST /api/auth/login {email,password}
  RR → Go POST /login → AT_browser + RT
  RR → Go POST /session/access {RT} → AT_ssr
  RR → SET rr:at:{sid}=AT_ssr (redis-rr)
  RR → Set-Cookie HttpOnly RT + JSON {accessToken: AT_browser, user…}
Browser keeps AT_browser in memory
```

## Silent browser AT renew (Session 5)

```text
Browser → RR POST /api/auth/refresh  (cookie RT, no body required)
  RR → Go POST /session/access {refreshToken}
  RR → JSON {accessToken} only
  No Set-Cookie; Go RT session unchanged; does NOT call Go /refresh
```

Route name `/api/auth/refresh` is FE-compat only; backend hop is **`/session/access`**.

## Optional forced RT rotation

```text
Client/tooling → Go POST /refresh {refreshToken}
  → new AT + new RT (old RT revoked)
```

Not used by browser silent renew. If RR ever exposes forced rotate, it must rewrite the cookie.

## SSR auth-gate loader

```text
Browser GET /app/...
  RR reads RT cookie
  RR → Go POST /session {refreshToken}  (204 or 401; no rotation)
  On success: render; SSR AT via getServerAccessToken (cache hit or /session/access)
```

## Domain API

```text
Browser → Traefik Authorization: Bearer AT_browser
  Traefik ForwardAuth → Go GET /check
  Traefik → wallet/transfer/inbox with X-User-Id
```

---

# 5. Token lifecycle

| Token | Storage | TTL (default) | Rotate on silent renew? |
|---|---|---|---|
| AT_browser | Browser memory | ~15m (`auth.jwt_ttl`) | Re-minted via `/session/access` |
| AT_ssr | `redis-rr` `rr:at:{sid}` | ~15m (`RR_AT_TTL_SECONDS`) | Re-minted on cache miss |
| RT | RR cookie + Go `auth:rt:{sid}` | ~1d (`auth.refresh_ttl`) | **No** on silent path; yes on Go `/refresh` |

Logout: revoke this Go RT + `DEL rr:at:{sid}` + clear cookie. Other devices keep their sessions.

---

# 6. Security

- AT never in LocalStorage / SessionStorage / IndexedDB
- RT: HttpOnly, SameSite=Lax, Secure when `COOKIE_SECURE=true`
- Go auth never sets browser cookies (JSON only)
- Two Redis instances: do not share Go auth Redis with RR SSR AT cache
- CSRF: same-site BFF + SameSite=Lax; Origin checks optional hardening

---

# 7. Scaling

- RR Node is horizontally scalable; SSR AT state is in `redis-rr` (not sticky sessions)
- Go auth RT store is shared Redis (`auth:rt:*`)
- Domain APIs remain stateless behind Traefik + ForwardAuth

---

# 8. Sequence (login + silent renew)

```text
Login:
  B → RR /api/auth/login
  RR → Go /login
  RR → Go /session/access
  RR → redis-rr SET rr:at
  RR → B (Set-Cookie RT + AT_browser JSON)

Silent renew:
  B → RR /api/auth/refresh (cookie)
  RR → Go /session/access
  RR → B (AT JSON, no Set-Cookie)

Domain:
  B → Traefik Bearer AT → ForwardAuth /check → API
```

---

# 9. Out of scope (still)

- Full API BFF for wallet/transfer/inbox
- Domain data loaded only via SSR
- Dual JWT `aud` (browser vs SSR)
- Logout-all devices

---

# 10. Resolved design questions

| Question | Decision |
|---|---|
| Cache SSR AT? | Yes — `redis-rr` `rr:at:{sid}`, TTL ≈ JWT |
| AT TTL | 15m (config) |
| Silent renew rotates RT? | **No** — Go `/session/access` only |
| Dedicated `/session`? | Yes — probe without rotation |
| Proactive vs 401 refresh | Client may refresh on 401 via BFF; path is still `/session/access` |
