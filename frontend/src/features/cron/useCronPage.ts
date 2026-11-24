import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import type { SegmentedOption } from '../../components/ui/SegmentedTabs'
import { loadState } from '../../hooks/loadState'
import { getCron, inspectCron } from '../../lib/api/cron'
import {
  directorySchedule,
  filterCronEntries,
  periodLabel,
  sortTimers,
  timerResult,
} from '../../lib/cron'
import { useRefreshInterval } from '../../lib/refresh'
import { cronExpressionSchema, type CronExpressionValues } from '../../schemas/cronExpression'

export type CronTab = 'cron' | 'timers' | 'inspector'

/** Logic and view model of CronPage (SPEC_ADMINISTRATION.md § 8), read-only. */
export function useCronPage() {
  const refreshInterval = useRefreshInterval()
  const [tab, setTab] = useState<CronTab>('cron')
  const [search, setSearch] = useState('')
  // The expression last submitted to the inspector.
  const [inspected, setInspected] = useState<string | null>(null)

  const cronQuery = useQuery({
    queryKey: ['cron'],
    queryFn: getCron,
    refetchInterval: refreshInterval || false,
  })
  const inspectQuery = useQuery({
    queryKey: ['cron', 'inspect', inspected],
    queryFn: () => inspectCron(inspected ?? ''),
    enabled: inspected !== null,
    retry: false,
  })
  const form = useForm<CronExpressionValues>({
    resolver: zodResolver(cronExpressionSchema),
    defaultValues: { expression: '0 3 * * *' },
  })

  const data = cronQuery.data
  const entries = useMemo(() => filterCronEntries(data?.entries ?? [], search), [data, search])
  const timers = useMemo(() => sortTimers(data?.timers ?? []), [data])
  const tabs: ReadonlyArray<SegmentedOption<CronTab>> = [
    { value: 'cron', label: 'Cron', count: data?.entries.length },
    { value: 'timers', label: 'Timers systemd', count: data?.timers.length },
    { value: 'inspector', label: 'Inspecteur' },
  ]

  return {
    state: loadState(cronQuery),
    loaded: data !== undefined,
    timeZone: data?.timeZone ?? '',
    tab,
    setTab,
    tabs,
    search,
    setSearch,
    entries,
    total: data?.entries.length ?? 0,
    journalAvailable: data?.journalAvailable ?? true,
    unreadable: data?.unreadable ?? [],
    directories: (data?.directories ?? []).map((directory) => ({
      path: directory.path,
      label: periodLabel(directory.period),
      scripts: directory.scripts,
      schedule: directorySchedule(directory, data?.anacron ?? false),
      nextRun: directory.scheduledBy?.nextRuns[0] ?? null,
    })),
    timers: timers.map((timer) => ({ ...timer, result: timerResult(timer) })),
    timersAvailable: data?.timersAvailable ?? false,
    inspector: {
      register: form.register,
      errors: form.formState.errors,
      onSubmit: form.handleSubmit((values) => setInspected(values.expression)),
      state: loadState(inspectQuery),
      result: inspected !== null ? (inspectQuery.data ?? null) : null,
    },
    refresh: () => void cronQuery.refetch(),
  }
}

export type CronPageModel = ReturnType<typeof useCronPage>
