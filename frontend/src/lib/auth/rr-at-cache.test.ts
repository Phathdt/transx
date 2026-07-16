import { describe, expect, it } from 'vitest'

// parseSessionId is pure; re-implement the contract here so unit tests do not
// pull ioredis into the jsdom suite. Keep in sync with app/lib/rr-at-cache.server.ts.
function parseSessionId(refreshToken: string): string | null {
  const value = refreshToken.trim()
  if (!value) return null
  const dot = value.indexOf('.')
  if (dot <= 0 || dot === value.length - 1) return null
  const sessionID = value.slice(0, dot)
  return sessionID || null
}

describe('parseSessionId', () => {
  it('extracts session id from RT', () => {
    expect(parseSessionId('abc123.secretpart')).toBe('abc123')
  })

  it('returns null for empty or malformed RT', () => {
    expect(parseSessionId('')).toBeNull()
    expect(parseSessionId('   ')).toBeNull()
    expect(parseSessionId('nosecret')).toBeNull()
    expect(parseSessionId('.secret')).toBeNull()
    expect(parseSessionId('sid.')).toBeNull()
  })

  it('keeps secret dots after first separator', () => {
    expect(parseSessionId('sid.sec.ret')).toBe('sid')
  })
})
