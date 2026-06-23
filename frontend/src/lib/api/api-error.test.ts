import { describe, expect, it } from 'vitest'
import { toApiError } from '#/lib/api/api-error'

function makeAxiosError(status: number, data: unknown) {
  return {
    isAxiosError: true,
    message: 'Request failed with status code ' + status,
    response: { status, data },
  }
}

describe('toApiError', () => {
  it('extracts backend error body fields', () => {
    const err = toApiError(
      makeAxiosError(409, {
        error: 'idempotency key reuse',
        requestId: 'req-1',
        traceId: 'trace-1',
      }),
    )
    expect(err.status).toBe(409)
    expect(err.message).toBe('idempotency key reuse')
    expect(err.requestId).toBe('req-1')
    expect(err.traceId).toBe('trace-1')
  })

  it('falls back to axios message when body has no error', () => {
    const err = toApiError(makeAxiosError(500, {}))
    expect(err.status).toBe(500)
    expect(err.message).toContain('500')
  })

  it('handles plain Error', () => {
    const err = toApiError(new Error('boom'))
    expect(err.status).toBe(0)
    expect(err.message).toBe('boom')
  })

  it('handles unknown values', () => {
    const err = toApiError('weird')
    expect(err.status).toBe(0)
    expect(err.message).toBe('Unexpected error')
  })
})
