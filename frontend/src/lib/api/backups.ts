import { requestJSON } from './client'

/** Backups with restic, increment 6A (API_CONTRACTS.md § 4.10). */
export type RepositoryType = 'local' | 'rest' | 's3'
export type BackupFrequency = 'daily' | 'weekly'
export type BackupOperation = 'init' | 'backup'
export type BackupOrigin = 'manual' | 'schedule' | 'catchup'
export type BackupRunState = 'running' | 'succeeded' | 'failed'
export type BackupFailure =
  | 'runner_not_installed'
  | 'not_allowed'
  | 'restic_failed'
  | 'timeout'
  | 'invalid_result'
  | 'secrets_unavailable'
  | 'job_failed'
  | 'systemd_unavailable'
  | 'interrupted'

export type BackupRepository = { type: RepositoryType; location: string }

/** Schedule kept by the Manager; time is HH:MM in the machine's time zone, weekday 0 is Sunday. */
export type BackupSchedule = {
  enabled: boolean
  frequency: BackupFrequency
  weekday: number
  time: string
}

export type BackupConfig = {
  repository: BackupRepository | null
  initialized: boolean
  initializedAt: string | null
  /** Whether each secret is set; secrets are never served. */
  credentials: { password: boolean; username: boolean; secret: boolean }
  secretsAvailable: boolean
  sources: string[]
  excludes: string[]
  excludeCaches: boolean
  excludeIfPresent: string
  schedule: BackupSchedule
  updatedAt: string | null
  updatedBy: string | null
}

/** username and secret: null keeps them, '' removes them. */
export type BackupConfigInput = {
  repository: BackupRepository | null
  username: string | null
  secret: string | null
  sources: string[]
  excludes: string[]
  excludeCaches: boolean
  excludeIfPresent: string
  schedule: BackupSchedule
}

export type BackupStats = {
  filesNew: number
  filesChanged: number
  filesUnmodified: number
  dirsNew: number
  dirsChanged: number
  dirsUnmodified: number
  dataAdded: number
  totalFilesProcessed: number
  totalBytesProcessed: number
  durationSeconds: number
}

export type BackupRun = {
  id: string
  operation: BackupOperation
  origin: BackupOrigin
  requestedBy: string | null
  state: BackupRunState
  failure: BackupFailure | null
  message: string | null
  warning: boolean
  snapshotId: string | null
  stats: BackupStats | null
  startedAt: string
  finishedAt: string | null
}

export type BackupSnapshot = {
  id: string
  shortId: string
  time: string
  hostname: string
  paths: string[]
  tags: string[]
  totalFiles: number
  totalBytes: number
  dataAdded: number
}

export type BackupSummary = {
  configured: boolean
  initialized: boolean
  running: BackupRun | null
  lastRun: BackupRun | null
  lastSuccess: BackupRun | null
  nextRunAt: string | null
}

export function getBackupSummary() {
  return requestJSON<BackupSummary>('/api/v1/backups/summary')
}

export function getBackupConfig() {
  return requestJSON<BackupConfig>('/api/v1/backups/config')
}

export function updateBackupConfig(input: BackupConfigInput) {
  return requestJSON<BackupConfig>('/api/v1/backups/config', {
    method: 'PUT',
    body: JSON.stringify(input),
  })
}

/** Starts restic init; the password is returned this once. */
export function initBackupRepository() {
  return requestJSON<{ password: string; run: BackupRun }>('/api/v1/backups/repository/init', {
    method: 'POST',
  })
}

export async function requestBackup() {
  const response = await requestJSON<{ run: BackupRun }>('/api/v1/backups/runs', { method: 'POST' })
  return response.run
}

export async function listBackupRuns(limit = 50) {
  const response = await requestJSON<{ runs: BackupRun[] }>(`/api/v1/backups/runs?limit=${limit}`)
  return response.runs
}

export async function listBackupSnapshots() {
  const response = await requestJSON<{ snapshots: BackupSnapshot[] }>('/api/v1/backups/snapshots')
  return response.snapshots
}
