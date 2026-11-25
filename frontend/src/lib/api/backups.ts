import { requestJSON } from './client'

/** Backups with restic, increments 6A to 6C (API_CONTRACTS.md § 4.10). */
export type RepositoryType = 'local' | 'rest' | 's3' | 'sftp'
export type BackupFrequency = 'daily' | 'weekly'
export type BackupOperation =
  | 'init'
  | 'backup'
  | 'forget-prune'
  | 'check'
  | 'restore'
  | 'snapshots'
  | 'unlock'
  | 'diff'
  | 'copy'
  | 'restore-in-place'
  | 'dump'

/** Operations an administrator starts (POST /api/v1/backups/operations). */
export type AdminBackupOperation =
  'forget-prune' | 'check' | 'unlock' | 'restore' | 'restore-in-place' | 'dump' | 'diff' | 'copy'

/** restic's modifiers: added, removed, content modified, metadata updated, type changed, bitrot. */
export type BackupDiffModifier = '+' | '-' | 'M' | 'U' | 'T' | '?'
export type BackupDiffCounts = { files: number; dirs: number; others: number; bytes: number }

/** Changes from the snapshot from to the snapshot to; changes holds the first 1 000. */
export type BackupDiff = {
  from: string
  to: string
  changedFiles: number
  added: BackupDiffCounts
  removed: BackupDiffCounts
  /** Empty for an account that cannot manage the backups. */
  changes: Array<{ path: string; modifier: BackupDiffModifier }>
  totalChanges: number
  truncated: boolean
}

/** What a copy brought to the copy repository. */
export type BackupCopyResult = {
  repository: BackupRepository
  initialized?: boolean
  snapshotsCopied: number
  snapshotsInTarget: number
  snapshotsRemoved?: number
}

export type BackupRetention = {
  enabled: boolean
  keepDaily: number
  keepWeekly: number
  keepMonthly: number
  keepYearly: number
}

export type BackupCheck = { enabled: boolean; subsetPercent: number }

/** Figures of a restore, a forget or a check. */
export type BackupRunDetails = {
  snapshotId?: string
  target?: string
  dryRun?: boolean
  /** Restore to the original location (D-2) and the backup made before it. */
  inPlace?: boolean
  safetySnapshotId?: string
  /** Archive of a dump run: the path archived and its size. */
  archivePath?: string
  archiveBytes?: number
  totalFiles?: number
  filesRestored?: number
  totalBytes?: number
  bytesRestored?: number
  snapshotsRemoved?: number
  subsetPercent?: number
  diff?: BackupDiff
  copy?: BackupCopyResult
}

export type BackupRestore = { snapshotId: string; include: string[]; dryRun: boolean }
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

/**
 * hostKey: the pinned host key of an SFTP server ("type base64"), sent as
 * pasted and stored normalized; hostKeyFingerprint is answered only.
 */
export type BackupRepository = {
  type: RepositoryType
  location: string
  hostKey?: string
  hostKeyFingerprint?: string
}

/** Public part of the SSH key of the Manager, for its SFTP repositories. */
export type BackupSSHKey = { publicKey: string; fingerprint: string; createdAt: string }

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
  /** SQLite files copied consistently before each backup (D-3). */
  sqliteDatabases: string[]
  schedule: BackupSchedule
  retention: BackupRetention
  check: BackupCheck
  updatedAt: string | null
  updatedBy: string | null
  /** The copy repository (restic copy), null when none is set. */
  copy: BackupCopyConfig | null
  /** The SSH key of the Manager, null until generated. */
  sshKey: BackupSSHKey | null
}

export type BackupCopyConfig = {
  repository: BackupRepository
  initialized: boolean
  initializedAt: string | null
  credentials: { password: boolean; username: boolean; secret: boolean }
  /** Copies after each successful scheduled backup. */
  afterBackup: boolean
}

/** username and secret: null keeps them, '' removes them. */
export type BackupCopyInput = {
  repository: BackupRepository
  username: string | null
  secret: string | null
  afterBackup: boolean
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
  sqliteDatabases: string[]
  schedule: BackupSchedule
  retention: BackupRetention
  check: BackupCheck
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
  details: BackupRunDetails | null
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
  lastCheck: BackupRun | null
  lastCopy: BackupRun | null
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

export async function requestBackupOperation(
  operation: AdminBackupOperation,
  options: {
    restore?: BackupRestore
    diff?: { from: string; to: string }
    dump?: { snapshotId: string; path: string }
  } = {},
) {
  const response = await requestJSON<{ run: BackupRun }>('/api/v1/backups/operations', {
    method: 'POST',
    body: JSON.stringify({ operation, ...options }),
  })
  return response.run
}

export async function getBackupRun(id: string) {
  const response = await requestJSON<{ run: BackupRun }>(
    `/api/v1/backups/runs/${encodeURIComponent(id)}`,
  )
  return response.run
}

export function setBackupCopy(input: BackupCopyInput) {
  return requestJSON<BackupConfig>('/api/v1/backups/copy', {
    method: 'PUT',
    body: JSON.stringify(input),
  })
}

/** Creates or replaces the SSH key of the Manager for its SFTP repositories. */
export function generateBackupSSHKey() {
  return requestJSON<BackupConfig>('/api/v1/backups/ssh-key', { method: 'POST' })
}

/** Forgets the copy repository; its data stays where it is. */
export function removeBackupCopy() {
  return requestJSON<BackupConfig>('/api/v1/backups/copy/remove', { method: 'POST' })
}

/** Creates the copy repository and copies; the password is returned this once. */
export function initBackupCopy() {
  return requestJSON<{ password: string; run: BackupRun }>('/api/v1/backups/copy/init', {
    method: 'POST',
  })
}

/** Attaches an existing copy repository with its password, by a first copy. */
export async function connectBackupCopy(password: string) {
  const response = await requestJSON<{ run: BackupRun }>('/api/v1/backups/copy/connect', {
    method: 'POST',
    body: JSON.stringify({ password }),
  })
  return response.run
}

/** Attaches an existing repository with its password. */
export async function connectBackupRepository(password: string) {
  const response = await requestJSON<{ run: BackupRun }>('/api/v1/backups/repository/connect', {
    method: 'POST',
    body: JSON.stringify({ password }),
  })
  return response.run
}
