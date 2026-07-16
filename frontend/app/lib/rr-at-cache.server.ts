/**
 * Server-only SSR access-token cache on dedicated RR Redis.
 * Key: rr:at:{sessionID} → raw JWT. Never import from client bundles.
 */
import Redis from 'ioredis'

const KEY_PREFIX = 'rr:at:'
const DEFAULT_TTL_SECONDS = 900

let client: Redis | null = null

function ttlSeconds(): number {
  const raw = process.env.RR_AT_TTL_SECONDS
  if (!raw) return DEFAULT_TTL_SECONDS
  const n = Number.parseInt(raw, 10)
  return Number.isFinite(n) && n > 0 ? n : DEFAULT_TTL_SECONDS
}

function getClient(): Redis {
  if (!client) {
    client = new Redis(process.env.RR_REDIS_URL ?? 'redis://localhost:16380', {
      maxRetriesPerRequest: 1,
      enableReadyCheck: true,
      lazyConnect: true,
    })
  }
  return client
}

async function withRedis<T>(fn: (r: Redis) => Promise<T>): Promise<T> {
  const r = getClient()
  if (r.status === 'wait' || r.status === 'end') {
    await r.connect()
  }
  return fn(r)
}

function cacheKey(sessionID: string): string {
  return `${KEY_PREFIX}${sessionID}`
}

/** RT format is "{sessionID}.{secret}". Returns sessionID or null. */
export function parseSessionId(refreshToken: string): string | null {
  const value = refreshToken.trim()
  if (!value) return null
  const dot = value.indexOf('.')
  if (dot <= 0 || dot === value.length - 1) return null
  return value.slice(0, dot)
}

export async function putServerAccessToken(
  sessionID: string,
  accessToken: string,
): Promise<void> {
  if (!sessionID || !accessToken) return
  await withRedis((r) => r.set(cacheKey(sessionID), accessToken, 'EX', ttlSeconds()))
}

export async function getServerAccessTokenBySession(
  sessionID: string,
): Promise<string | null> {
  if (!sessionID) return null
  return withRedis((r) => r.get(cacheKey(sessionID)))
}

export async function deleteServerAccessToken(sessionID: string): Promise<void> {
  if (!sessionID) return
  await withRedis((r) => r.del(cacheKey(sessionID)))
}

/** Test helper — reset singleton between unit tests. */
export function __resetRrAtCacheForTests(): void {
  if (client) {
    client.disconnect()
    client = null
  }
}
