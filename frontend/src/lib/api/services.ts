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
  /** Allow list of the service configuration (SPEC_ADMINISTRATION.md § 5.2), sorted. */
  manageableUnits: string[]
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

/** The four changes the Manager may ask systemd for (API_CONTRACTS.md § 4.7). */
export type UnitVerb = 'start' | 'stop' | 'restart' | 'reload'

export type UnitActionState = 'running' | 'succeeded' | 'failed'

export type UnitActionFailure =
  'failed' | 'not_allowed' | 'not_found' | 'rejected' | 'timeout' | 'unavailable' | 'interrupted'

/** The last action requested on a unit since the Manager started. */
export type UnitAction = {
  unit: string
  action: UnitVerb
  state: UnitActionState
  failure: UnitActionFailure | null
  requestedBy: string
  requestedAt: string
  finishedAt: string | null
  /** Unit state once stable; null while running or when it could not be read. */
  final: { activeState: string; subState: string } | null
}

export async function requestUnitAction(name: string, action: UnitVerb) {
  const response = await requestJSON<{ action: UnitAction }>(
    `/api/v1/system/units/${encodeURIComponent(name)}/actions`,
    { method: 'POST', body: JSON.stringify({ action }) },
  )
  return response.action
}

export async function getLastUnitAction(name: string) {
  const response = await requestJSON<{ action: UnitAction }>(
    `/api/v1/system/units/${encodeURIComponent(name)}/actions/last`,
  )
  return response.action
}
