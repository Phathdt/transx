import { describe, expect, it } from 'vitest'
import {
  formatFailureReason,
  isTerminalStatus,
} from '#/lib/transfer/transfer-status'

describe('isTerminalStatus', () => {
  it('treats lifecycle statuses as non-terminal', () => {
    for (const s of ['PENDING', 'RESERVED', 'PROCESSING', 'SUBMITTED']) {
      expect(isTerminalStatus(s)).toBe(false)
    }
  })

  it('treats settled statuses as terminal', () => {
    for (const s of ['SUCCEEDED', 'FAILED', 'REVERSED', 'UNKNOWN']) {
      expect(isTerminalStatus(s)).toBe(true)
    }
  })

  it('returns false for undefined/empty', () => {
    expect(isTerminalStatus(undefined)).toBe(false)
    expect(isTerminalStatus('')).toBe(false)
  })
})

describe('formatFailureReason', () => {
  it('maps known reasons to readable copy', () => {
    expect(formatFailureReason('INSUFFICIENT_FUNDS')).toBe('Insufficient funds')
    expect(formatFailureReason('PROVIDER_REJECTED')).toBe(
      'Payment provider rejected the transfer',
    )
  })

  it('falls back to the raw value for unknown reasons', () => {
    expect(formatFailureReason('SOME_NEW_REASON')).toBe('SOME_NEW_REASON')
  })

  it('returns null when absent', () => {
    expect(formatFailureReason(undefined)).toBeNull()
  })
})
