import { useState } from 'react'
import { Search } from 'lucide-react'
import { lookupAccount } from '#/lib/api/generated/wallet/wallet'
import type { DtoAccountLookupResponse } from '#/lib/api/generated/models'
import { toApiError } from '#/lib/api/api-error'
import { Button } from '#/components/ui/button'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'

export type AccountType = 'internal' | 'external'

const INTERNAL_REF = /^ACC-[0-9A-HJKMNP-TV-Z]{26}$/
const EXTERNAL_REF = /^EXT-[A-Za-z0-9-]{1,64}$/

function validateRef(type: AccountType, ref: string): string | null {
  if (!ref) return 'Account reference is required'
  if (type === 'internal' && !INTERNAL_REF.test(ref)) {
    return 'Internal ref must look like ACC-XXXXXXXXXXXXXXXXXXXXXXXXXX'
  }
  if (type === 'external' && !EXTERNAL_REF.test(ref)) {
    return 'External ref must look like EXT-...'
  }
  return null
}

interface AccountLookupFieldProps {
  type: AccountType
  onTypeChange: (type: AccountType) => void
  value: string
  onValueChange: (value: string) => void
  onResolved: (result: DtoAccountLookupResponse | null) => void
}

/**
 * Destination account lookup. The caller owns the type + ref state; this field
 * validates by type and calls the lookup endpoint on explicit action/blur, then
 * reports the resolved beneficiary (no balances shown).
 */
export function AccountLookupField({
  type,
  onTypeChange,
  value,
  onValueChange,
  onResolved,
}: AccountLookupFieldProps) {
  const [result, setResult] = useState<DtoAccountLookupResponse | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [isLoading, setIsLoading] = useState(false)

  function reset() {
    setResult(null)
    setError(null)
    onResolved(null)
  }

  async function runLookup() {
    const validationError = validateRef(type, value)
    if (validationError) {
      setError(validationError)
      setResult(null)
      onResolved(null)
      return
    }

    setIsLoading(true)
    setError(null)
    try {
      const data = await lookupAccount(type, value)
      setResult(data)
      onResolved(data)
    } catch (err) {
      const apiError = toApiError(err)
      const message =
        apiError.status === 502
          ? 'Beneficiary lookup is temporarily unavailable. Try again.'
          : apiError.message
      setError(message)
      setResult(null)
      onResolved(null)
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <div className="space-y-2">
      <Label htmlFor="toAccountRef">Destination account</Label>
      <div className="flex gap-2">
        <Select
          value={type}
          onValueChange={(next) => {
            onTypeChange(next as AccountType)
            reset()
          }}
        >
          <SelectTrigger className="w-32" aria-label="Account type">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="internal">Internal</SelectItem>
            <SelectItem value="external">External</SelectItem>
          </SelectContent>
        </Select>
        <Input
          id="toAccountRef"
          value={value}
          placeholder={type === 'internal' ? 'ACC-…' : 'EXT-…'}
          onChange={(e) => {
            onValueChange(e.target.value)
            reset()
          }}
          onBlur={() => {
            if (value) runLookup()
          }}
          aria-invalid={Boolean(error)}
        />
        <Button
          type="button"
          variant="outline"
          onClick={runLookup}
          disabled={isLoading}
        >
          <Search className="size-4" />
          {isLoading ? 'Looking…' : 'Lookup'}
        </Button>
      </div>

      {error ? <p className="text-sm text-destructive">{error}</p> : null}

      {result ? (
        <div className="rounded-md border bg-muted/40 p-3 text-sm">
          <div className="flex items-center justify-between">
            <span className="text-muted-foreground">Holder</span>
            <span className="font-medium">{result.holderName}</span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-muted-foreground">Account</span>
            <span className="font-medium">{result.accountRef}</span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-muted-foreground">Currency</span>
            <span className="font-medium">{result.currency}</span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-muted-foreground">Status</span>
            <span className="font-medium">{result.status}</span>
          </div>
        </div>
      ) : null}
    </div>
  )
}
