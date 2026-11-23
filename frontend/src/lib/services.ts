import type { BadgeTone } from '../components/ui/Badge'
import type { SystemUnit, UnitLimit, UnitType, UnitsStatus } from './api/services'
import { formatBytes, formatDecimal, formatNumber } from './format'

export type UnitTypeFilter = 'all' | UnitType
export type UnitStateFilter = 'all' | 'active' | 'failed' | 'other'

export type UnitFilters = { type: UnitTypeFilter; state: UnitStateFilter; search: string }

export const unitTypeOptions: ReadonlyArray<{ value: UnitTypeFilter; label: string }> = [
  { value: 'all', label: 'Tous les types' },
  { value: 'service', label: 'Services' },
  { value: 'timer', label: 'Timers' },
  { value: 'socket', label: 'Sockets' },
]

export function isUnitTypeFilter(value: string): value is UnitTypeFilter {
  return unitTypeOptions.some((option) => option.value === value)
}

function stateGroup(activeState: string): Exclude<UnitStateFilter, 'all'> {
  if (activeState === 'active') return 'active'
  if (activeState === 'failed') return 'failed'
  return 'other'
}

/** Units matching the filters, failed first then by name (SPEC_ADMINISTRATION.md § 5.1). */
export function filterUnits(units: SystemUnit[], filters: UnitFilters): SystemUnit[] {
  const needle = filters.search.trim().toLowerCase()
  return units
    .filter((unit) => filters.type === 'all' || unit.type === filters.type)
    .filter((unit) => filters.state === 'all' || stateGroup(unit.activeState) === filters.state)
    .filter(
      (unit) =>
        !needle ||
        unit.name.toLowerCase().includes(needle) ||
        unit.description.toLowerCase().includes(needle),
    )
    .sort((a, b) => {
      const failedA = a.activeState === 'failed'
      const failedB = b.activeState === 'failed'
      if (failedA !== failedB) return failedA ? -1 : 1
      return a.name.localeCompare(b.name)
    })
}

export function unitStateCounts(units: SystemUnit[]): Record<UnitStateFilter, number> {
  const counts = { all: units.length, active: 0, failed: 0, other: 0 }
  for (const unit of units) counts[stateGroup(unit.activeState)]++
  return counts
}

const stateLabels: Record<string, string> = {
  active: 'Actif',
  failed: 'En échec',
  inactive: 'Inactif',
  activating: 'Démarrage',
  deactivating: 'Arrêt',
  reloading: 'Rechargement',
  maintenance: 'Maintenance',
}

export function unitStateLabel(activeState: string): string {
  return stateLabels[activeState] ?? activeState
}

export function unitStateTone(activeState: string): BadgeTone {
  switch (activeState) {
    case 'active':
      return 'success'
    case 'failed':
      return 'danger'
    case 'activating':
    case 'deactivating':
    case 'reloading':
    case 'maintenance':
      return 'warning'
    default:
      return 'neutral'
  }
}

export function unitTypeLabel(type: UnitType): string {
  return { service: 'Service', timer: 'Timer', socket: 'Socket' }[type]
}

const fileStateLabels: Record<string, string> = {
  enabled: 'Activé',
  'enabled-runtime': 'Activé (temporaire)',
  disabled: 'Désactivé',
  static: 'Statique',
  masked: 'Masqué',
  generated: 'Généré',
  indirect: 'Indirect',
  transient: 'Transitoire',
  alias: 'Alias',
}

export function unitFileStateLabel(state: string | null): string {
  if (!state) return '—'
  return fileStateLabels[state] ?? state
}

export function unitsStatusInfo(status: UnitsStatus): { label: string; tone: BadgeTone } {
  switch (status) {
    case 'ok':
      return { label: 'À jour', tone: 'success' }
    case 'error':
      return { label: 'Erreur de collecte', tone: 'danger' }
    case 'unavailable':
      return { label: 'systemd indisponible', tone: 'warning' }
    default:
      return { label: 'Collecte en cours', tone: 'neutral' }
  }
}

/** CPU time consumed by a service; null (accounting disabled) is a dash. */
export function formatCpuTime(nanoseconds: number | null): string {
  if (nanoseconds == null || !Number.isFinite(nanoseconds)) return '—'
  const seconds = nanoseconds / 1e9
  // Below a second, milliseconds: 0.02 s must not read as 0.
  if (seconds > 0 && seconds < 1)
    return `${formatNumber(Math.max(1, Math.round(nanoseconds / 1e6)))} ms`
  if (seconds < 60) return `${formatDecimal(seconds)} s`
  const total = Math.floor(seconds)
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  if (hours > 0) return `${formatNumber(hours)} h ${String(minutes).padStart(2, '0')} min`
  return `${minutes} min ${total % 60} s`
}

export function formatUnitLimit(limit: UnitLimit, kind: 'count' | 'bytes' = 'count'): string {
  if (limit.unlimited) return 'illimité'
  if (limit.value == null) return 'inconnu'
  return kind === 'bytes' ? formatBytes(limit.value) : formatNumber(limit.value)
}

/** The journal excerpt comes newest first; it reads like journalctl, oldest first. */
export function oldestFirst<T>(entries: T[]): T[] {
  return [...entries].reverse()
}
