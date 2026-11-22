import { loadState } from '../../hooks/loadState'
import { useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { listEvents, type MonitorEvent } from '../../lib/api/monitoring'
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

  const events = useQuery({
    queryKey: ['events', range],
    queryFn: () => listEvents({ from: rangeStart(range), to: new Date(), limit: 500 }),
    refetchInterval: refreshInterval || false,
  })

  const all = useMemo(() => events.data ?? [], [events.data])
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
    all,
    counts,
    filtered,
  }
}
