import { zodResolver } from '@hookform/resolvers/zod'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { loadState } from '../../hooks/loadState'
import {
  getJournalEntry,
  getJournalSummary,
  journalExportUrl,
  listJournal,
  listJournalBoots,
  listJournalUnits,
  type JournalEntry,
  type JournalFilter,
  type JournalPriorityName,
  type JournalSource,
} from '../../lib/api/journal'
import {
  bootLabel,
  fieldsJson,
  isJournalPeriod,
  isJournalPriority,
  isJournalSource,
  journalPeriodOptions,
  journalPeriodStart,
  journalPriorityOptions,
  journalSourceOptions,
  mergeNewer,
  summaryChartOption,
  summaryTotals,
  toJournalRow,
  visibleEntries,
  type JournalPeriod,
} from '../../lib/journal'
import { useChartTheme } from '../../lib/theme'
import { journalSearchSchema, rangeBound, type JournalSearchValues } from '../../schemas/journal'

/** Interval of the live follow (SPEC_ADMINISTRATION.md § 4). */
const followIntervalMs = 2000
const followLimit = 500

/** Followed entries, newest first, and the cursor to follow from. */
type LiveEntries = { entries: JournalEntry[]; cursor: string | null }

const noLiveEntries: LiveEntries = { entries: [], cursor: null }

/** Logic and view model of JournalPage. */
export function useJournalPage() {
  const chartTheme = useChartTheme()
  const [source, setSource] = useState<JournalSource>('all')
  const [priority, setPriority] = useState<JournalPriorityName>('info')
  const [unit, setUnit] = useState('')
  const [boot, setBoot] = useState('')
  const [period, setPeriod] = useState<JournalPeriod>('24h')
  const [search, setSearch] = useState<JournalSearchValues>({ q: '', from: '', to: '' })
  // Reference time of the preset periods; a refresh moves it to now.
  const [anchor, setAnchor] = useState(() => new Date())
  const [follow, setFollow] = useState(false)
  const [selected, setSelected] = useState<string | null>(null)
  const queryClient = useQueryClient()

  const form = useForm<JournalSearchValues>({
    resolver: zodResolver(journalSearchSchema),
    defaultValues: { q: '', from: '', to: '' },
  })

  const filter = useMemo<JournalFilter>(
    () => ({
      source,
      priority,
      unit: source === 'docker' ? undefined : unit || undefined,
      boot: boot || undefined,
      from: period === 'custom' ? rangeBound(search.from) : journalPeriodStart(period, anchor),
      to: period === 'custom' ? rangeBound(search.to) : undefined,
      q: search.q || undefined,
    }),
    [source, priority, unit, boot, period, search, anchor],
  )
  const key = `${JSON.stringify(filter)}|${anchor.getTime()}`

  const pages = useInfiniteQuery({
    queryKey: ['journal', 'entries', key],
    queryFn: ({ pageParam }) => listJournal(filter, { before: pageParam }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.olderCursor ?? undefined,
  })

  // The follow query accumulates the newer entries of one filter in its own
  // data: each poll reads after the cursor of the previous one.
  const followKey = ['journal', 'follow', key]
  const followed = useQuery({
    queryKey: followKey,
    queryFn: async (): Promise<LiveEntries> => {
      const previous = queryClient.getQueryData<LiveEntries>(followKey) ?? noLiveEntries
      const after = previous.cursor ?? pages.data?.pages[0]?.newerCursor ?? undefined
      const page = await listJournal(filter, { after, limit: followLimit })
      return {
        entries: mergeNewer(previous.entries, page.entries),
        cursor: page.newerCursor ?? previous.cursor,
      }
    },
    enabled: follow && pages.isSuccess,
    // TanStack Query pauses the interval while the tab is hidden.
    refetchInterval: followIntervalMs,
  })
  const live = followed.data ?? noLiveEntries

  const rows = useMemo(
    () =>
      visibleEntries(live.entries, pages.data?.pages.map((page) => page.entries) ?? []).map(
        toJournalRow,
      ),
    [live.entries, pages.data],
  )
  const lastPage = pages.data?.pages[pages.data.pages.length - 1]

  const detail = useQuery({
    queryKey: ['journal', 'entry', selected],
    queryFn: () => getJournalEntry(selected ?? ''),
    enabled: selected !== null,
  })
  const summary = useQuery({
    queryKey: ['journal', 'summary'],
    queryFn: getJournalSummary,
    refetchInterval: 60_000,
  })
  const boots = useQuery({ queryKey: ['journal', 'boots'], queryFn: listJournalBoots })
  const units = useQuery({ queryKey: ['journal', 'units'], queryFn: listJournalUnits })

  const chartOption = useMemo(
    () => (summary.data ? summaryChartOption(summary.data, chartTheme) : null),
    [summary.data, chartTheme],
  )

  return {
    summary: {
      state: loadState(summary),
      chartOption,
      totals: summary.data ? summaryTotals(summary.data) : [],
      total: summary.data?.total ?? null,
      truncated: summary.data?.truncated ?? false,
      diskUsageBytes: summary.data?.diskUsageBytes ?? null,
      // Unknown until the summary loads: no warning meanwhile.
      limitedAccess: summary.data ? !summary.data.fullAccess : false,
    },
    source,
    setSource: (value: string) => {
      if (isJournalSource(value)) setSource(value)
    },
    sourceOptions: journalSourceOptions,
    priority,
    setPriority: (value: string) => {
      if (isJournalPriority(value)) setPriority(value)
    },
    priorityOptions: journalPriorityOptions,
    unit,
    setUnit,
    unitDisabled: source === 'docker',
    unitOptions: units.data ?? [],
    boot,
    setBoot,
    bootOptions: (boots.data ?? []).map((item) => ({ value: item.bootId, label: bootLabel(item) })),
    period,
    setPeriod: (value: string) => {
      if (isJournalPeriod(value)) setPeriod(value)
    },
    periodOptions: journalPeriodOptions,
    customRange: period === 'custom',
    register: form.register,
    errors: form.formState.errors,
    onSearch: form.handleSubmit((values) => setSearch(values)),
    entries: {
      state: loadState(pages),
      rows,
      filtered: search.q !== '',
      hasOlder: pages.hasNextPage,
      loadOlder: () => void pages.fetchNextPage(),
      isLoadingOlder: pages.isFetchingNextPage,
      scanLimitReached: lastPage?.scanLimitReached ?? false,
      skipped: (pages.data?.pages ?? []).reduce((sum, page) => sum + page.skipped, 0),
    },
    follow,
    toggleFollow: () => setFollow((value) => !value),
    followError: followed.isError ? followed.error.message : null,
    // A new reference time is a new filter: pages and live entries restart.
    refresh: () => setAnchor(new Date()),
    selected,
    select: (cursor: string) => setSelected(cursor),
    closeInspector: () => setSelected(null),
    inspector: {
      state: loadState(detail),
      entry: detail.data ? toJournalRow(detail.data.entry) : null,
      fields: detail.data?.fields ?? [],
      json: detail.data ? fieldsJson(detail.data.fields) : '',
    },
    exportLogUrl: journalExportUrl(filter, 'log'),
    exportJsonUrl: journalExportUrl(filter, 'json'),
  }
}

export type JournalPageModel = ReturnType<typeof useJournalPage>
