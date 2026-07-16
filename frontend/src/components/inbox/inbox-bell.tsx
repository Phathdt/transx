import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Bell } from 'lucide-react'
import {
  getGetInboxUnreadCountQueryKey,
  getListInboxQueryKey,
  useGetInboxUnreadCount,
  useListInbox,
  useMarkAllInboxRead,
} from '#/lib/api/generated/inbox/inbox'
import type {
  DtoInboxItemResponse,
  DtoInboxListResponse,
  DtoUnreadCountResponse,
} from '#/lib/api/generated/models'
import { Button } from '#/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '#/components/ui/popover'
import { formatInboxTime } from './format-inbox-time'
import { InboxDetailSheet } from './inbox-detail-sheet'

const POLL_MS = 20_000

function badgeLabel(count: number): string {
  if (count <= 0) return ''
  return count > 9 ? '9+' : String(count)
}

export type InboxBellProps = {
  /** SSR seed from layout loader (AT_ssr). */
  initialUnreadCount?: number
  /**
   * True once AT_browser is ready (silent renew). Until then show SSR badge
   * only — do not hit Traefik without a bearer.
   */
  clientReady?: boolean
}

/**
 * Header bell: SSR unread for first paint, then React Query polls with AT_browser.
 * Opening the popover loads the inbox list; item clicks open a detail sheet.
 */
export function InboxBell({
  initialUnreadCount = 0,
  clientReady = false,
}: InboxBellProps) {
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  // Stable "fetchedAt" for SSR seed so RQ does not treat layout data as age 0.
  const [ssrSeededAt] = useState(() => Date.now())

  const initialUnread: DtoUnreadCountResponse = { count: initialUnreadCount }

  const { data: unread } = useGetInboxUnreadCount<DtoUnreadCountResponse>({
    query: {
      // SSR layout loader seeds the badge; client must not re-hit unread-count
      // on reload/mount just because AT_browser became ready.
      initialData: initialUnread,
      initialDataUpdatedAt: ssrSeededAt,
      staleTime: POLL_MS,
      enabled: clientReady,
      refetchInterval: clientReady ? POLL_MS : false,
      refetchOnWindowFocus: clientReady,
      refetchOnMount: false,
      refetchOnReconnect: clientReady,
    },
  })
  const count = unread?.count ?? initialUnreadCount

  const { data: listData, isLoading } = useListInbox<DtoInboxListResponse>(
    { page: 1, pageSize: 20 },
    {
      query: {
        enabled: open && clientReady,
        refetchOnWindowFocus: clientReady,
      },
    },
  )
  const items: DtoInboxItemResponse[] = listData?.data ?? []

  const markAll = useMarkAllInboxRead({
    mutation: {
      onSuccess: async () => {
        await Promise.all([
          queryClient.invalidateQueries({
            queryKey: getGetInboxUnreadCountQueryKey(),
          }),
          queryClient.invalidateQueries({ queryKey: getListInboxQueryKey() }),
        ])
      },
    },
  })

  return (
    <>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            className="relative"
            aria-label={
              count > 0 ? `Inbox, ${count} unread` : 'Inbox, no unread'
            }
          >
            <Bell className="size-4" />
            {count > 0 ? (
              <span className="absolute -top-0.5 -right-0.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-destructive px-1 text-[10px] font-semibold text-white">
                {badgeLabel(count)}
              </span>
            ) : null}
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-96 p-0" align="end">
          <div className="flex items-center justify-between border-b px-3 py-2">
            <p className="text-sm font-semibold">Notifications</p>
            <Button
              type="button"
              variant="ghost"
              size="xs"
              disabled={!clientReady || count === 0 || markAll.isPending}
              onClick={() => markAll.mutate()}
            >
              Mark all read
            </Button>
          </div>
          <div className="max-h-80 overflow-y-auto">
            {!clientReady ? (
              <p className="px-3 py-6 text-center text-sm text-muted-foreground">
                Loading session…
              </p>
            ) : isLoading ? (
              <p className="px-3 py-6 text-center text-sm text-muted-foreground">
                Loading…
              </p>
            ) : items.length === 0 ? (
              <p className="px-3 py-6 text-center text-sm text-muted-foreground">
                No notifications yet
              </p>
            ) : (
              <ul className="divide-y">
                {items.map((item) => {
                  const unreadItem = item.readAt == null
                  return (
                    <li key={item.id}>
                      <button
                        type="button"
                        className="flex w-full flex-col gap-0.5 px-3 py-2.5 text-left hover:bg-accent/60"
                        onClick={() => {
                          if (item.id) {
                            setSelectedId(item.id)
                            setOpen(false)
                          }
                        }}
                      >
                        <div className="flex items-start gap-2">
                          {unreadItem ? (
                            <span
                              className="mt-1.5 size-1.5 shrink-0 rounded-full bg-primary"
                              aria-hidden
                            />
                          ) : (
                            <span className="mt-1.5 size-1.5 shrink-0" />
                          )}
                          <div className="min-w-0 flex-1">
                            <p
                              className={
                                unreadItem
                                  ? 'truncate text-sm font-semibold'
                                  : 'truncate text-sm font-medium text-muted-foreground'
                              }
                            >
                              {item.title}
                            </p>
                            <p className="line-clamp-2 text-xs text-muted-foreground">
                              {item.body}
                            </p>
                            <p className="mt-0.5 text-[11px] text-muted-foreground">
                              {formatInboxTime(item.createdAt)}
                            </p>
                          </div>
                        </div>
                      </button>
                    </li>
                  )
                })}
              </ul>
            )}
          </div>
        </PopoverContent>
      </Popover>

      <InboxDetailSheet
        itemId={selectedId}
        onOpenChange={(next) => {
          if (!next) setSelectedId(null)
        }}
      />
    </>
  )
}
