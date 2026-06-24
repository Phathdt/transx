import { useEffect, useMemo, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useNavigate } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import {
  createTransfer,
  getListTransfersQueryKey,
  useListAccounts,
} from '#/lib/api/generated/wallet/wallet'
import type {
  CreateTransferBody,
  DtoAccountLookupResponse,
} from '#/lib/api/generated/models'
import { toApiError } from '#/lib/api/api-error'
import { useAuth } from '#/hooks/use-auth'
import { useTransferIdempotencyKey } from '#/hooks/use-transfer-idempotency-key'
import { AccountLookupField } from './account-lookup-field'
import type { AccountType } from './account-lookup-field'
import { Button } from '#/components/ui/button'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'
import { Card, CardContent } from '#/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'
import { Alert, AlertDescription, AlertTitle } from '#/components/ui/alert'

// amount: positive decimal, max 4 fraction digits, max 16 integer digits.
const AMOUNT_RE = /^\d{1,16}(\.\d{1,4})?$/

const formSchema = z.object({
  fromAccountRef: z.string().min(1, 'Select a source account'),
  amount: z
    .string()
    .min(1, 'Amount is required')
    .regex(AMOUNT_RE, 'Enter a positive amount with up to 4 decimals')
    .refine((v) => Number(v) > 0, 'Amount must be greater than zero'),
  currency: z.string().min(1, 'Currency is required'),
  message: z.string().min(1, 'Message is required').max(255, 'Message too long'),
})

type FormValues = z.infer<typeof formSchema>

export function CreateTransferPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const idempotencyKey = useTransferIdempotencyKey()
  const { userName } = useAuth()

  const [transferType, setTransferType] = useState<AccountType>('internal')
  const [toAccountRef, setToAccountRef] = useState('')
  const [resolved, setResolved] = useState<DtoAccountLookupResponse | null>(
    null,
  )
  const [submitError, setSubmitError] = useState<string | null>(null)
  const [isSubmitting, setIsSubmitting] = useState(false)
  // Once the user edits the message, stop overwriting it with the template so a
  // later re-lookup does not clobber their edit.
  const messageDirty = useRef(false)

  const { data: accountsData, isLoading: accountsLoading } = useListAccounts({
    pageSize: 100,
  })
  const accounts = accountsData?.data ?? []

  const {
    register,
    handleSubmit,
    watch,
    setValue,
    formState: { errors },
  } = useForm<FormValues>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      fromAccountRef: '',
      amount: '',
      currency: '',
      message: '',
    },
  })

  const fromAccountRef = watch('fromAccountRef')
  const selectedAccount = useMemo(
    () => accounts.find((a) => a.accountRef === fromAccountRef),
    [accounts, fromAccountRef],
  )

  // Receiver display name for the template: the looked-up holder name, falling
  // back to the entered destination ref/beneficiary when no name is resolved.
  const receiverName = resolved?.holderName || toAccountRef

  // Pre-fill the message with a "{sender} transfer to {receiver}" template until
  // the user edits it. Recomputes as sender/receiver change.
  useEffect(() => {
    if (messageDirty.current) return
    const sender = userName || 'You'
    const receiver = receiverName || 'recipient'
    setValue('message', `${sender} transfer to ${receiver}`)
  }, [userName, receiverName, setValue])

  async function onSubmit(values: FormValues) {
    setSubmitError(null)

    if (transferType === 'internal' && !toAccountRef) {
      setSubmitError('Internal transfers require a destination account.')
      return
    }
    if (transferType === 'internal' && toAccountRef === values.fromAccountRef) {
      setSubmitError('Source and destination accounts must differ.')
      return
    }

    const body: CreateTransferBody = {
      fromAccountRef: values.fromAccountRef,
      amount: values.amount,
      currency: values.currency,
      transferType: transferType.toUpperCase(),
      message: values.message,
      ...(toAccountRef ? { toAccountRef } : {}),
    }

    // Fresh idempotency key per new submit attempt.
    const key = idempotencyKey.rotate()
    setIsSubmitting(true)
    try {
      const result = await createTransfer(body, {
        headers: { 'Idempotency-Key': key },
      })
      // Refresh the list cache so the new transfer shows on return.
      queryClient.invalidateQueries({ queryKey: getListTransfersQueryKey() })
      toast.success('Transfer created')
      navigate({
        to: '/app/transfers/$transferId',
        params: { transferId: result.transferId ?? '' },
      })
    } catch (err) {
      const apiError = toApiError(err)
      setSubmitError(
        apiError.status === 409
          ? 'This idempotency key was already used with a different transfer. Refresh and try again.'
          : apiError.message,
      )
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="mx-auto max-w-xl">
      <div className="mb-6">
        <p className="island-kicker mb-1">Move money</p>
        <h1 className="display-title text-3xl font-bold text-[var(--sea-ink)]">
          New Transfer
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Send funds between wallets or to an external account.
        </p>
      </div>
      <Card className="glass-card border-0 shadow-none">
        <CardContent className="pt-6">
          <form
            onSubmit={handleSubmit(onSubmit)}
            className="space-y-5"
            noValidate
          >
            {submitError ? (
              <Alert variant="destructive">
                <AlertTitle>Transfer failed</AlertTitle>
                <AlertDescription>{submitError}</AlertDescription>
              </Alert>
            ) : null}

            <div className="space-y-2">
              <Label htmlFor="fromAccountRef">Source account</Label>
              <Select
                value={fromAccountRef}
                onValueChange={(v) => {
                  setValue('fromAccountRef', v, { shouldValidate: true })
                  const acc = accounts.find((a) => a.accountRef === v)
                  if (acc?.currency) {
                    setValue('currency', acc.currency, { shouldValidate: true })
                  }
                }}
                disabled={accountsLoading}
              >
                <SelectTrigger
                  id="fromAccountRef"
                  aria-invalid={Boolean(errors.fromAccountRef)}
                >
                  <SelectValue
                    placeholder={
                      accountsLoading
                        ? 'Loading accounts…'
                        : 'Select an account'
                    }
                  />
                </SelectTrigger>
                <SelectContent>
                  {accounts.map((acc) => (
                    <SelectItem
                      key={acc.accountRef}
                      value={acc.accountRef ?? ''}
                    >
                      {acc.accountRef} · {acc.currency}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {errors.fromAccountRef ? (
                <p className="text-sm text-destructive">
                  {errors.fromAccountRef.message}
                </p>
              ) : null}
              {selectedAccount ? (
                <p className="text-xs text-muted-foreground tabular-nums">
                  Available: {selectedAccount.availableBalance}{' '}
                  {selectedAccount.currency}
                </p>
              ) : null}
            </div>

            <AccountLookupField
              type={transferType}
              onTypeChange={setTransferType}
              value={toAccountRef}
              onValueChange={setToAccountRef}
              onResolved={setResolved}
            />
            {transferType === 'external' && !resolved ? (
              <p className="text-xs text-muted-foreground">
                External destination is optional to validate; the provider is
                set server-side.
              </p>
            ) : null}

            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-2">
                <Label htmlFor="amount">Amount</Label>
                <Input
                  id="amount"
                  inputMode="decimal"
                  placeholder="0.00"
                  aria-invalid={Boolean(errors.amount)}
                  {...register('amount')}
                />
                {errors.amount ? (
                  <p className="text-sm text-destructive">
                    {errors.amount.message}
                  </p>
                ) : null}
              </div>
              <div className="space-y-2">
                <Label htmlFor="currency">Currency</Label>
                <Input
                  id="currency"
                  placeholder="USD"
                  aria-invalid={Boolean(errors.currency)}
                  {...register('currency')}
                />
                {errors.currency ? (
                  <p className="text-sm text-destructive">
                    {errors.currency.message}
                  </p>
                ) : null}
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor="message">Message</Label>
              <Input
                id="message"
                placeholder="Add a note for this transfer"
                aria-invalid={Boolean(errors.message)}
                {...register('message', {
                  onChange: () => {
                    messageDirty.current = true
                  },
                })}
              />
              {errors.message ? (
                <p className="text-sm text-destructive">
                  {errors.message.message}
                </p>
              ) : null}
            </div>

            <Button type="submit" className="w-full" disabled={isSubmitting}>
              {isSubmitting ? 'Submitting…' : 'Create transfer'}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
