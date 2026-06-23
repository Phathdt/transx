import { useCallback, useRef } from 'react'

function newKey(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return crypto.randomUUID()
  }
  // Fallback for environments without crypto.randomUUID.
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`
}

/**
 * Manages the Idempotency-Key for transfer creation. A new key is minted per
 * fresh submit; the same key is reused only when retrying the identical payload
 * after a network failure (so the backend replays instead of double-charging).
 */
export function useTransferIdempotencyKey() {
  const keyRef = useRef<string | null>(null)

  // Mint a new key for a brand-new submit attempt.
  const rotate = useCallback(() => {
    keyRef.current = newKey()
    return keyRef.current
  }, [])

  // Reuse the current key (retry of same payload), minting one if absent.
  const current = useCallback(() => {
    if (!keyRef.current) keyRef.current = newKey()
    return keyRef.current
  }, [])

  return { rotate, current }
}
