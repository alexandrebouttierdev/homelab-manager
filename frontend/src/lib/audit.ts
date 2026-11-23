import type { AuditEntry } from './api/security'
import { formatDateTime } from './format'

type Tone = 'success' | 'danger' | 'warning' | 'info' | 'neutral'

const outcomes: Record<string, { label: string; tone: Tone }> = {
  success: { label: 'Réussi', tone: 'success' },
  denied: { label: 'Refusé', tone: 'danger' },
  failure: { label: 'Échec', tone: 'warning' },
  requested: { label: 'Demandé', tone: 'info' },
}

export const auditOutcomeOptions = [
  { value: '', label: 'Tous les résultats' },
  ...Object.entries(outcomes).map(([value, outcome]) => ({ value, label: outcome.label })),
]

export type AuditPeriod = '24h' | '7d' | '30d' | 'all'

export const auditPeriodOptions: Array<{ value: AuditPeriod; label: string }> = [
  { value: '24h', label: '24 heures' },
  { value: '7d', label: '7 jours' },
  { value: '30d', label: '30 jours' },
  { value: 'all', label: 'Tout' },
]

const periodMilliseconds: Record<Exclude<AuditPeriod, 'all'>, number> = {
  '24h': 24 * 60 * 60 * 1000,
  '7d': 7 * 24 * 60 * 60 * 1000,
  '30d': 30 * 24 * 60 * 60 * 1000,
}

/** Start of the period, or undefined for the whole log. */
export function auditPeriodStart(period: AuditPeriod, now: Date): Date | undefined {
  if (period === 'all') return undefined
  return new Date(now.getTime() - periodMilliseconds[period])
}

export type AuditRow = {
  id: string
  createdAt: string
  actor: string
  action: string
  target: string
  outcomeLabel: string
  outcomeTone: Tone
  requestId: string
}

export function toAuditRow(entry: AuditEntry): AuditRow {
  const outcome = outcomes[entry.outcome] ?? { label: entry.outcome, tone: 'neutral' as const }
  return {
    id: entry.id,
    createdAt: formatDateTime(entry.createdAt),
    actor: entry.actor,
    action: entry.action,
    target: `${entry.targetType} · ${entry.targetId}`,
    outcomeLabel: outcome.label,
    outcomeTone: outcome.tone,
    requestId: entry.requestId ?? '—',
  }
}

export function isAuditPeriod(value: string): value is AuditPeriod {
  return auditPeriodOptions.some((option) => option.value === value)
}
