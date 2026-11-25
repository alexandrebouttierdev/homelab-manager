import type { BackupSchedule } from './backups'
import { requestJSON } from './client'

/** A task of the maintenance catalog (API_CONTRACTS.md § 4.13, decision D-1). */
export type MaintenanceTask = {
  kind: 'journal-vacuum'
  schedule: BackupSchedule
  maxSizeMiB: number
  /** Sizes the journal may be kept under, in MiB. */
  sizes: number[]
  unit: string
  running: boolean
  nextRunAt: string | null
  lastRun: {
    at: string
    origin: 'manual' | 'schedule'
    succeeded: boolean
    message: string | null
  } | null
  updatedAt: string | null
  updatedBy: string | null
}

export async function listMaintenanceTasks() {
  const body = await requestJSON<{ tasks: MaintenanceTask[] }>('/api/v1/maintenance/tasks')
  return body.tasks
}

export async function updateMaintenanceTask(
  kind: MaintenanceTask['kind'],
  input: { schedule: BackupSchedule; maxSizeMiB: number },
) {
  const body = await requestJSON<{ task: MaintenanceTask }>(
    `/api/v1/maintenance/tasks/${encodeURIComponent(kind)}`,
    { method: 'PUT', body: JSON.stringify(input) },
  )
  return body.task
}

export function runMaintenanceTask(kind: MaintenanceTask['kind']) {
  return requestJSON<{ requested: boolean }>(
    `/api/v1/maintenance/tasks/${encodeURIComponent(kind)}/run`,
    { method: 'POST' },
  )
}
