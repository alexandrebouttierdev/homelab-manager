import { requestJSON } from './client'

export type JournalSource = 'all' | 'kernel' | 'auth' | 'docker'
export type JournalPriorityName =
  'emerg' | 'alert' | 'crit' | 'err' | 'warning' | 'notice' | 'info' | 'debug'

/** Filters of the system journal (API_CONTRACTS.md § 4.6). */
export type JournalFilter = {
  source: JournalSource
  priority: JournalPriorityName
  unit?: string
  boot?: string
  from?: string
  to?: string
  q?: string
}

export type JournalEntry = {
  cursor: string
  time: string
  priority: number | null
  unit: string
  identifier: string
  pid: string
  hostname: string
  bootId: string
  message: string
  truncated: boolean
}

export type JournalPage = {
  entries: JournalEntry[]
  olderCursor: string | null
  newerCursor: string | null
  scanned: number
  scanLimitReached: boolean
  skipped: number
}

export type JournalField = { name: string; value: string; masked: boolean }

export type JournalEntryDetail = { entry: JournalEntry; fields: JournalField[] }

export type JournalBoot = { index: number; bootId: string; firstEntry: string; lastEntry: string }

export type JournalSummary = {
  start: string
  bucketMinutes: number
  buckets: { start: string; counts: number[] }[]
  total: number
  truncated: boolean
  diskUsageBytes: number | null
  fullAccess: boolean
  generatedAt: string
}

/** Query parameters of a filter; the server defaults are left out. */
export function journalQuery(filter: JournalFilter): URLSearchParams {
  const query = new URLSearchParams()
  if (filter.source !== 'all') query.set('source', filter.source)
  if (filter.priority !== 'info') query.set('priority', filter.priority)
  for (const name of ['unit', 'boot', 'from', 'to', 'q'] as const) {
    const value = filter[name]
    if (value) query.set(name, value)
  }
  return query
}

export type JournalPageRequest = { before?: string; after?: string; limit?: number }

/** One page of entries, newest first; URLSearchParams escapes the cursors. */
export function listJournal(filter: JournalFilter, page: JournalPageRequest = {}) {
  const query = journalQuery(filter)
  if (page.before) query.set('before', page.before)
  if (page.after) query.set('after', page.after)
  if (page.limit) query.set('limit', String(page.limit))
  const text = query.toString()
  return requestJSON<JournalPage>(`/api/v1/journal/entries${text ? `?${text}` : ''}`)
}

export function getJournalEntry(cursor: string) {
  return requestJSON<JournalEntryDetail>(
    `/api/v1/journal/entry?${new URLSearchParams({ cursor }).toString()}`,
  )
}

export function listJournalBoots() {
  return requestJSON<{ boots: JournalBoot[] }>('/api/v1/journal/boots').then((body) => body.boots)
}

export function listJournalUnits() {
  return requestJSON<{ units: string[] }>('/api/v1/journal/units').then((body) => body.units)
}

export function getJournalSummary() {
  return requestJSON<JournalSummary>('/api/v1/journal/summary')
}

/** Same-origin link that downloads the filtered journal. */
export function journalExportUrl(filter: JournalFilter, format: 'log' | 'json') {
  const query = journalQuery(filter)
  query.set('format', format)
  return `/api/v1/journal/export?${query.toString()}`
}
