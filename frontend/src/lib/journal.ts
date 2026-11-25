import type { EChartsOption } from 'echarts'
import type { BadgeTone } from '../components/ui/Badge'
import type {
  JournalBoot,
  JournalEntry,
  JournalField,
  JournalPriorityName,
  JournalSource,
  JournalSummary,
} from './api/journal'
import { axisTooltip } from './charts'
import { formatChartTime, formatCount, formatDateTime } from './format'
import type { useChartTheme } from './theme'

type ChartTheme = ReturnType<typeof useChartTheme>

/** Syslog priorities, from 0 (emerg) to 7 (debug). */
export const journalPriorities: { value: JournalPriorityName; label: string }[] = [
  { value: 'emerg', label: 'Urgence' },
  { value: 'alert', label: 'Alerte' },
  { value: 'crit', label: 'Critique' },
  { value: 'err', label: 'Erreur' },
  { value: 'warning', label: 'Avertissement' },
  { value: 'notice', label: 'Notice' },
  { value: 'info', label: 'Information' },
  { value: 'debug', label: 'Débogage' },
]

/** Options of the minimum priority filter: a priority and the more severe ones. */
export const journalPriorityOptions = journalPriorities.map((priority, index) => ({
  value: priority.value,
  label: index === 0 ? priority.label : `${priority.label} et plus grave`,
}))

export function isJournalPriority(value: string): value is JournalPriorityName {
  return journalPriorities.some((priority) => priority.value === value)
}

export const journalSourceOptions: { value: JournalSource; label: string }[] = [
  { value: 'all', label: 'Tout le journal' },
  { value: 'kernel', label: 'Noyau' },
  { value: 'auth', label: 'Authentification' },
  { value: 'docker', label: 'Démon Docker' },
]

export function isJournalSource(value: string): value is JournalSource {
  return journalSourceOptions.some((source) => source.value === value)
}

export type JournalPeriod = '1h' | '24h' | '7d' | 'all' | 'custom'

export const journalPeriodOptions: { value: JournalPeriod; label: string }[] = [
  { value: '1h', label: 'Dernière heure' },
  { value: '24h', label: 'Dernières 24 h' },
  { value: '7d', label: '7 derniers jours' },
  { value: 'all', label: 'Tout' },
  { value: 'custom', label: 'Personnalisée' },
]

export function isJournalPeriod(value: string): value is JournalPeriod {
  return journalPeriodOptions.some((period) => period.value === value)
}

const periodHours: Partial<Record<JournalPeriod, number>> = { '1h': 1, '24h': 24, '7d': 168 }

/** Start of a preset period, as RFC 3339; undefined for all or custom. */
export function journalPeriodStart(period: JournalPeriod, now: Date): string | undefined {
  const hours = periodHours[period]
  return hours === undefined ? undefined : new Date(now.getTime() - hours * 3_600_000).toISOString()
}

export function priorityLabel(priority: number | null): string {
  return priority === null ? 'Sans priorité' : (journalPriorities[priority]?.label ?? 'Inconnue')
}

export function priorityTone(priority: number | null): BadgeTone {
  if (priority === null) return 'neutral'
  if (priority <= 3) return 'danger'
  if (priority === 4) return 'warning'
  if (priority === 5) return 'info'
  return 'neutral'
}

const escape = 0x1b

/**
 * Text of a message without terminal escape sequences (colours, cursor
 * moves) nor control characters other than tab and line feed, which a
 * terminal would interpret.
 */
export function displayMessage(message: string): string {
  let text = ''
  for (let index = 0; index < message.length; index++) {
    const code = message.charCodeAt(index)
    if (code === escape) {
      if (message[index + 1] === '[') {
        // Control sequence: parameters and intermediates up to a final byte.
        index += 2
        while (index < message.length) {
          const byte = message.charCodeAt(index)
          if (byte >= 0x40 && byte <= 0x7e) break
          index++
        }
      } else {
        index++
      }
      continue
    }
    if ((code < 0x20 && code !== 0x09 && code !== 0x0a) || code === 0x7f) continue
    text += message[index]
  }
  return text
}

const journalTimeFormat = new Intl.DateTimeFormat('fr-FR', {
  day: '2-digit',
  month: 'short',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
})

export function formatJournalTime(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '—' : journalTimeFormat.format(date)
}

/** Source shown for an entry: its unit, else its identifier. */
export function entrySource(entry: JournalEntry): string {
  return entry.unit || entry.identifier || '—'
}

export type JournalRow = {
  cursor: string
  time: string
  timeLabel: string
  priorityLabel: string
  priorityTone: BadgeTone
  source: string
  identifier: string
  message: string
  truncated: boolean
}

export function toJournalRow(entry: JournalEntry): JournalRow {
  return {
    cursor: entry.cursor,
    time: entry.time,
    timeLabel: formatJournalTime(entry.time),
    priorityLabel: priorityLabel(entry.priority),
    priorityTone: priorityTone(entry.priority),
    source: entrySource(entry),
    identifier: entry.pid ? `${entry.identifier}[${entry.pid}]` : entry.identifier,
    message: displayMessage(entry.message),
    truncated: entry.truncated,
  }
}

/** Live entries kept on screen at most; older ones are read again on demand. */
export const maxLiveEntries = 1000

/**
 * Puts newer entries (newest first) above the live ones, without duplicates,
 * keeping at most maxLiveEntries.
 */
export function mergeNewer(live: JournalEntry[], newer: JournalEntry[]): JournalEntry[] {
  const seen = new Set<string>()
  const merged: JournalEntry[] = []
  for (const entry of [...newer, ...live]) {
    if (seen.has(entry.cursor)) continue
    seen.add(entry.cursor)
    merged.push(entry)
    if (merged.length === maxLiveEntries) break
  }
  return merged
}

/** Live entries above the pages read, without duplicates. */
export function visibleEntries(live: JournalEntry[], pages: JournalEntry[][]): JournalEntry[] {
  const seen = new Set<string>()
  return [...live, ...pages.flat()].filter((entry) => {
    if (seen.has(entry.cursor)) return false
    seen.add(entry.cursor)
    return true
  })
}

export function bootLabel(boot: JournalBoot): string {
  const since = formatDateTime(boot.firstEntry)
  if (boot.index === 0) return `Démarrage actuel (depuis ${since})`
  return `Démarrage ${boot.index} (${since} → ${formatDateTime(boot.lastEntry)})`
}

/** Raw JSON of an entry, as the inspector shows it (masked values included). */
export function fieldsJson(fields: JournalField[]): string {
  return JSON.stringify(
    Object.fromEntries(fields.map((field) => [field.name, field.value])),
    null,
    2,
  )
}

/** Priority groups of the 24-hour chart, from the most severe. */
export const summaryGroups = [
  { label: 'Erreurs et plus grave', priorities: [0, 1, 2, 3], color: 'danger' },
  { label: 'Avertissements', priorities: [4], color: 'warning' },
  { label: 'Notices', priorities: [5], color: 'accent' },
  { label: 'Informations et débogage', priorities: [6, 7], color: 'muted' },
] as const

export type SummaryTotal = { label: string; total: number }

export function summaryTotals(summary: JournalSummary): SummaryTotal[] {
  return summaryGroups.map((group) => ({
    label: group.label,
    total: summary.buckets.reduce(
      (sum, bucket) =>
        sum + group.priorities.reduce<number>((part, p) => part + (bucket.counts[p] ?? 0), 0),
      0,
    ),
  }))
}

/** Stacked bars of the 24-hour summary, one bar per 15 minutes. */
export function summaryChartOption(summary: JournalSummary, theme: ChartTheme): EChartsOption {
  return {
    backgroundColor: 'transparent',
    tooltip: axisTooltip(theme, (value) => formatCount(value), { shadow: true }),
    legend: {
      bottom: 0,
      textStyle: { color: theme.text, fontSize: 11 },
      itemWidth: 10,
      itemHeight: 10,
    },
    grid: { left: 40, right: 8, top: 10, bottom: 46 },
    xAxis: {
      type: 'category',
      data: summary.buckets.map((bucket) => bucket.start),
      axisTick: { show: false },
      axisLine: { lineStyle: { color: theme.axis } },
      axisLabel: {
        color: theme.muted,
        fontSize: 10,
        formatter: (value: string) => formatChartTime(value),
      },
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      axisLabel: { color: theme.muted, fontSize: 10 },
      splitLine: { lineStyle: { color: theme.grid } },
    },
    series: summaryGroups.map((group) => ({
      name: group.label,
      type: 'bar',
      stack: 'priorities',
      barCategoryGap: '20%',
      itemStyle: { color: theme[group.color] },
      data: summary.buckets.map((bucket) =>
        group.priorities.reduce<number>((sum, p) => sum + (bucket.counts[p] ?? 0), 0),
      ),
    })),
  }
}
