/**
 * Orval server mutator (fetch). RR loaders/actions only — not client bundles.
 * Base URL = Go auth/API (AUTH_API_BASE_URL), not the browser BFF.
 *
 * Signature matches Orval `client: 'fetch'` + custom mutator:
 *   serverApiClient<T>(url, init?) => Promise<T>
 */

export function authApiBaseURL(): string {
  return (
    process.env.AUTH_API_BASE_URL ??
    process.env.VITE_API_BASE_URL ??
    'http://localhost:4000/api/v1'
  )
}

function resolveURL(path: string): string {
  if (/^https?:\/\//i.test(path)) return path
  const base = authApiBaseURL().replace(/\/$/, '')
  const pathPart = path.startsWith('/') ? path : `/${path}`
  return `${base}${pathPart}`
}

/**
 * Returns response body. Throws `Response` on non-2xx so BFF routes can
 * rethrow status/text the same way as the previous hand-written helpers.
 */
export async function serverApiClient<T>(
  url: string,
  init?: RequestInit,
): Promise<T> {
  const headers = new Headers(init?.headers)
  if (init?.body != null && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  const res = await fetch(resolveURL(url), {
    ...init,
    headers,
  })

  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new Response(text, { status: res.status })
  }

  if (res.status === 204 || res.status === 205 || res.status === 304) {
    return undefined as T
  }

  const text = await res.text()
  if (!text) return undefined as T
  return JSON.parse(text) as T
}

export default serverApiClient
