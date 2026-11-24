import { requestJSON } from './client'
import type { JournalEntry } from './journal'

export type UnitType = 'service' | 'timer' | 'socket'

/** Status of the units collection (API_CONTRACTS.md § 4.7). */
export type UnitsStatus = 'pending' | 'ok' | 'error' | 'unavailable'

/** One unit of the inventory; null is a value systemd does not give, never 0. */
export type SystemUnit = {
  name: string
  description: string
  type: UnitType
  loadState: string
  activeState: string
  subState: string
  unitFileState: string | null
  stateChangedAt: string | null
  mainPid: number | null
  memoryBytes: number | null
  cpuNanoseconds: number | null
  result: string | null
  nextElapse: string | null
  lastTrigger: string | null
  triggers: string[]
}

export type UnitCounts = { total: number; active: number; failed: number; activeTimers: number }

export type UnitInventory = {
  status: UnitsStatus
  collectedAt: string | null
  counts: UnitCounts
  units: SystemUnit[]
}

/** A limit: unlimited is systemd's "infinity"; value null and not unlimited is unknown. */
export type UnitLimit = { value: number | null; unlimited: boolean }

export type UnitDefinition = {
  fragmentPath: string | null
  execStart: string[]
  requires: string[]
  after: string[]
  restart: string | null
  slice: string | null
  tasksMax: UnitLimit
  limitNofile: UnitLimit
  memoryMax: UnitLimit
}

export type UnitDetail = {
  unit: SystemUnit
  definition: UnitDefinition
  /** Newest entries of the unit journal, newest first. */
  journal: { available: boolean; entries: JournalEntry[] }
}

export function listUnits() {
  return requestJSON<UnitInventory>('/api/v1/system/units')
}

export function getUnit(name: string) {
  return requestJSON<UnitDetail>(`/api/v1/system/units/${encodeURIComponent(name)}`)
}
