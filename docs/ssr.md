# PRD: Hybrid Authentication Architecture for React Router v7 + Go Backend

## Status

**Implemented (pivot A)** — Auth BFF on React Router Node; Go auth is JSON tokens only.

### Implemented decisions

| Item | Choice |
|---|---|
| FE runtime | React Router v7 framework mode (`app/`, SSR) |
| Access token | Memory only (browser); never web storage |
| Refresh token | Redis-backed opaque session on **Go**; **HttpOnly cookie on RR host** |
| Topology | `Browser → RR Node (cookie RT) → Go Auth (JSON AT+RT)` |
| Domain APIs | Browser → Traefik with **Bearer AT** (not full BFF) |
| Cookie SameSite | `Lax` on FE origin (same-site auth BFF routes) |
| SSR loaders | Auth-gate only: cookie → Go `POST /session` (no rotation) |
| Client AT obtain | Silent `POST /api/auth/refresh` (RR BFF) + 401 single-flight retry |
| AT TTL / RT TTL | 15m / 30d (config) |
| Out of scope | Full API BFF, server data loaders, transfer form actions |

Go public auth (no ForwardAuth): `/login`, `/refresh`, `/logout`, `/session` — **JSON body**, no `Set-Cookie`.  
RR BFF routes: `/api/auth/login|refresh|logout` own the cookie.

---

# 1. Overview

This document defines the authentication architecture for a web application using:

- React Router v7 (Framework/Server Mode)
- Go Backend
- JWT Authentication
- Refresh Token Rotation
- Hybrid browser+SSR cookie model (not a full BFF for domain APIs)

The goal is to support both:

- Direct API calls from the browser (Bearer AT)
- Server-side auth-gate loaders through React Router

without duplicating authentication logic.

---

# 2. Goals

### Functional Goals

- Support SSR and React Router loaders.
- Allow browser to call backend APIs directly.
- Support automatic token refresh.
- Avoid storing access tokens in LocalStorage.
- Minimize authentication state synchronization.
- Support horizontal scaling for frontend and backend.

### Non-functional Goals

- Stateless frontend servers whenever possible.
- Secure against XSS token theft.
- Easy deployment on Kubernetes.
- Compatible with CDN for static assets.
- Future support for multiple frontend applications.

---

# 3. High-Level Architecture

```text
                  Browser
                     │
                     │
         HttpOnly Refresh Cookie
                     │
        ┌────────────┴────────────┐
        │                         │
        ▼                         ▼
 Browser Runtime           React Router Server
 (Access Token A)          (Access Token B)
        │                         │
        └────────────┬────────────┘
                     ▼
                 Go Backend
```

The Refresh Token is the only shared authentication credential.

Access Tokens are independent.

---

# 4. Authentication Components

## Browser

Stores:

- Access Token (memory only)
- Refresh Token (HttpOnly Secure Cookie)

Responsibilities:

- Call backend APIs directly.
- Refresh Access Token automatically.
- Never persist Access Token.

---

## React Router Server

Stores:

- No persistent authentication state.

Responsibilities:

- Read Refresh Cookie.
- Obtain Access Token when required.
- Execute loaders/actions.
- Call backend APIs.
- Discard Access Token after request completes.

Optional optimization:

- Short-lived in-memory cache (1~5 minutes).

---

## Go Backend

Responsibilities:

- Login
- Logout
- Refresh Token Rotation
- JWT generation
- JWT validation
- Session invalidation

---

# 5. Authentication Flow

## Login

```text
Browser
    │
POST /login
    │
    ▼
Go Backend
    │
    ├── Set-Cookie(refresh_token)
    │
    └── JSON(access_token)
    │
    ▼
Browser
```

Browser stores:

- Access Token → Memory
- Refresh Token → Cookie

---

## Browser API Request

```text
Browser

Authorization: Bearer access_token

↓

Go Backend
```

---

## Browser Refresh

```text
Browser

↓

POST /refresh

↓

Cookie automatically included

↓

Go Backend

↓

New Access Token

↓

Browser Memory
```

---

## React Router Loader

```text
Browser

↓

GET /dashboard

↓

React Router Server

↓

Read Refresh Cookie

↓

POST /refresh (if needed)

↓

Access Token

↓

GET /dashboard-data

↓

Render HTML

↓

Browser
```

Access Token exists only during request execution.

---

# 6. Token Lifecycle

## Refresh Token

Storage:

- HttpOnly
- Secure
- SameSite=Lax (or Strict if possible)

Lifetime:

30~90 days

Rotation:

Enabled

Revocable:

Yes

---

## Access Token

Storage:

Memory only

Lifetime:

10~15 minutes

Revocable:

By expiration or backend blacklist (optional)

Persistent:

No

---

# 7. Authentication Strategy

## Browser

```text
Refresh Cookie
      │
      ▼
Access Token (Memory)
      │
      ▼
Authorization Header
```

---

## Server

```text
Refresh Cookie
      │
      ▼
Temporary Access Token
      │
      ▼
Authorization Header
```

Browser and Server never exchange Access Tokens.

---

# 8. Authorization Header

Every backend request uses:

```http
Authorization: Bearer <access_token>
```

The backend authentication mechanism remains identical regardless of caller.

---

# 9. Refresh Strategy

## Browser

Refresh when:

- Access Token expires.
- Backend returns HTTP 401.

---

## Server

Preferred:

Refresh only when Access Token is unavailable or expired.

Optional:

Cache temporary Access Token per session.

Maximum cache duration:

5 minutes.

---

# 10. Security Considerations

## Access Token

Never store in:

- LocalStorage
- SessionStorage
- IndexedDB

Reason:

Protection against XSS.

---

## Refresh Token

Must use:

- HttpOnly
- Secure
- SameSite=Lax (or Strict)

Never exposed to JavaScript.

---

## CSRF

Because cookies are used:

- Validate Origin/Referer for sensitive operations.
- Or implement CSRF Token.
- SameSite provides additional mitigation.

---

# 11. Horizontal Scaling

React Router Server should remain stateless.

Each request should be independently authenticated using the Refresh Cookie.

No sticky session required.

No shared frontend session store required.

---

# 12. Sequence Diagram

## Browser API

```text
Browser
    │
Authorization: Bearer Access Token
    │
    ▼
Go Backend
```

---

## SSR Request

```text
Browser
    │
Cookie
    ▼
React Router Server
    │
    ├── Refresh Access Token
    │
    ├── Call Backend
    │
    ▼
Go Backend
```

---

# 13. Advantages

- Secure authentication model.
- No Access Token persistence.
- SSR compatible.
- Browser and Server remain independent.
- Simple backend validation.
- Easy Kubernetes deployment.
- No frontend sticky sessions.
- Works with React Router loaders/actions.
- Supports both SSR and SPA interactions.

---

# 14. Future Enhancements

- Silent refresh scheduling.
- Multi-device session management.
- Device fingerprint tracking.
- Risk-based authentication.
- Session dashboard.
- OAuth2 / OpenID Connect integration.
- WebAuthn / Passkey support.
- Redis-backed short-lived Access Token cache (optional).
- Token introspection endpoint (optional).

---

# 15. Open Questions

1. Should the React Router Server cache temporary Access Tokens?
2. What should be the Access Token TTL (10 vs 15 minutes)?
3. Should Refresh Token rotation invalidate previous tokens immediately?
4. Should backend expose a dedicated `/session` endpoint?
5. Should browser proactively refresh tokens before expiration or only on HTTP 401?
