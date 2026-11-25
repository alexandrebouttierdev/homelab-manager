import { loadState } from '../../hooks/loadState'
import { useInfiniteQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { listEventsPage, type MonitorEvent } from '../../lib/api/monitoring'
import {
  eventPageSize,
  nextEventPage,
  type EventPageParam,
  type WindowedEventPage,
} from '../../lib/events'
import { useRefreshInterval } from '../../lib/refresh'

export type SeverityFilter = 'all' | 'info' | 'warning' | 'error'

export type RangeKey = '24h' | '7d' | '30d'

export const ranges: Array<{ value: RangeKey; label: string; milliseconds: number }> = [
  { value: '24h', label: '24 heures', milliseconds: 24 * 60 * 60 * 1000 },
  { value: '7d', label: '7 jours', milliseconds: 7 * 24 * 60 * 60 * 1000 },
  { value: '30d', label: '30 jours', milliseconds: 30 * 24 * 60 * 60 * 1000 },
]

export function rangeStart(range: RangeKey): Date {
  return new Date(
    Date.now() -
      (ranges.find((item) => item.value === range)?.milliseconds ?? ranges[0].milliseconds),
  )
}

export function severityCounts(events: MonitorEvent[]) {
  return {
    info: events.filter((event) => event.severity === 'info').length,
    warning: events.filter((event) => event.severity === 'warning').length,
    error: events.filter((event) => event.severity === 'error' || event.severity === 'critical')
      .length,
  }
}

/** Logic and view model of EventsPage. */
export function useEventsPage() {
  const refreshInterval = useRefreshInterval()
  const [range, setRange] = useState<RangeKey>('24h')
  const [severity, setSeverity] = useState<SeverityFilter>('all')
  const [search, setSearch] = useState('')

  // The first page fixes the window; the next ones read older events of
  // the same window after its cursor.
  const events = useInfiniteQuery({
    queryKey: ['events', range],
    queryFn: async ({ pageParam }): Promise<WindowedEventPage> => {
      const to = pageParam ? new Date(pageParam.to) : new Date()
      const from = pageParam ? new Date(pageParam.from) : rangeStart(range)
      const page = await listEventsPage({
        from,
        to,
        limit: eventPageSize,
        cursor: pageParam?.cursor,
      })
      return { ...page, from: from.getTime(), to: to.getTime() }
    },
    initialPageParam: null as EventPageParam | null,
    getNextPageParam: nextEventPage,
    refetchInterval: refreshInterval || false,
  })

  const all = useMemo(() => events.data?.pages.flatMap((page) => page.events) ?? [], [events.data])
  const counts = severityCounts(all)

  const filtered = useMemo(() => {
    return all.filter((event) => {
      if (severity === 'error' && event.severity !== 'error' && event.severity !== 'critical')
        return false
      if (severity !== 'all' && severity !== 'error' && event.severity !== severity) return false
      if (!search) return true
      const haystack = `${event.type} ${event.message} ${event.severity}`.toLowerCase()
      return haystack.includes(search.toLowerCase())
    })
  }, [all, search, severity])

  return {
    range,
    setRange,
    severity,
    setSeverity,
    search,
    setSearch,
    events: loadState(events),
    hasMore: events.hasNextPage,
    loadingMore: events.isFetchingNextPage,
    loadMore: () => void events.fetchNextPage(),
    all,
    counts,
    filtered,
  }
}
