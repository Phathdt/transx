import { useEffect } from 'react'
import { Link } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import {
  getGetInboxUnreadCountQueryKey,
  getListInboxQueryKey,
  useGetInboxItem,
} from '#/lib/api/generated/inbox/inbox'
import type { DtoInboxItemResponse } from '#/lib/api/generated/models'
import { Button } from '#/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '#/components/ui/sheet'
import { formatInboxTime } from './format-inbox-time'

type InboxDetailSheetProps = {
  itemId: string | null
  onOpenChange: (open: boolean) => void
}

/**
 * Detail panel for one inbox item. Opening the item calls GET /inbox/:id which
 * auto-marks unread rows as read on the server; we invalidate list/count so the
 * badge and bold state update.
 */
export function InboxDetailSheet({
  itemId,
  onOpenChange,
}: InboxDetailSheetProps) {
  const queryClient = useQueryClient()
  const open = itemId != null

  const { data, isLoading, isError, isSuccess } =
    useGetInboxItem<DtoInboxItemResponse>(itemId ?? '', {
      query: {
        enabled: open,
        // Re-fetch on each open so the auto-read path always runs once per open.
        staleTime: 0,
      },
    })

  useEffect(() => {
    if (!isSuccess || !data?.id) return
    void queryClient.invalidateQueries({
      queryKey: getGetInboxUnreadCountQueryKey(),
    })
    void queryClient.invalidateQueries({ queryKey: getListInboxQueryKey() })
  }, [isSuccess, data?.id, data?.readAt, queryClient])

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right">
        <SheetHeader>
          <SheetTitle>
            {data?.title ?? (isLoading ? 'Loading…' : 'Inbox')}
          </SheetTitle>
          <SheetDescription>
            {data?.createdAt ? formatInboxTime(data.createdAt) : ' '}
          </SheetDescription>
        </SheetHeader>
        <div className="flex flex-1 flex-col gap-4 px-4 pb-6">
          {isError ? (
            <p className="text-sm text-destructive">
              Could not load this notification.
            </p>
          ) : (
            <p className="text-sm leading-relaxed whitespace-pre-wrap text-foreground">
              {data?.body ?? (isLoading ? '…' : '')}
            </p>
          )}
          {data?.transferId ? (
            <Button asChild variant="outline" className="self-start">
              <Link
                to="/app/transfers/$transferId"
                params={{ transferId: data.transferId }}
                onClick={() => onOpenChange(false)}
              >
                View transfer
              </Link>
            </Button>
          ) : null}
        </div>
      </SheetContent>
    </Sheet>
  )
}
