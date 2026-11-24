import { describe, expect, it } from 'vitest'
import type { BackupConfig, BackupRun, BackupSummary } from './api/backups'
import {
  backupHealth,
  configToValues,
  describeDetails,
  credentialLabels,
  describeSchedule,
  describeStats,
  formatDuration,
  runFailureText,
  runLabel,
  runStateInfo,
  valuesToInput,
} from './backups'

function run(overrides: Partial<BackupRun> = {}): BackupRun {
  return {
    id: 'r1',
    operation: 'backup',
    origin: 'schedule',
    requestedBy: null,
    state: 'succeeded',
    failure: null,
    message: null,
    warning: false,
    snapshotId: 'abcdef0123',
    stats: null,
    details: null,
    startedAt: '2026-11-24T03:00:00Z',
    finishedAt: '2026-11-24T03:01:30Z',
    ...overrides,
  }
}

const config: BackupConfig = {
  repository: { type: 's3', location: 'https://s3.example.com/bucket' },
  initialized: true,
  initializedAt: '2026-11-24T00:00:00Z',
  credentials: { password: true, username: true, secret: true },
  secretsAvailable: true,
  sources: ['/etc', '/srv'],
  excludes: ['*.tmp'],
  excludeCaches: true,
  excludeIfPresent: '.nobackup',
  schedule: { enabled: true, frequency: 'weekly', weekday: 0, time: '04:30' },
  retention: { enabled: false, keepDaily: 7, keepWeekly: 4, keepMonthly: 6, keepYearly: 0 },
  check: { enabled: true, subsetPercent: 5 },
  updatedAt: null,
  updatedBy: null,
}

describe('backups', () => {
  it('describes the schedule in French', () => {
    expect(describeSchedule(config.schedule)).toBe('Chaque dimanche à 04:30')
    expect(describeSchedule({ ...config.schedule, frequency: 'daily' })).toBe('Chaque jour à 04:30')
    expect(describeSchedule({ ...config.schedule, enabled: false })).toBe('Désactivée')
  })

  it('labels runs, their state and their failure', () => {
    expect(runLabel(run())).toBe('Sauvegarde planifiée')
    expect(runLabel(run({ origin: 'catchup' }))).toBe('Sauvegarde rattrapage')
    expect(runLabel(run({ operation: 'init' }))).toBe('Initialisation du dépôt')
    expect(runStateInfo(run()).tone).toBe('success')
    expect(runStateInfo(run({ warning: true })).tone).toBe('warning')
    expect(runStateInfo(run({ state: 'failed' })).tone).toBe('danger')
    expect(
      runFailureText(
        run({ state: 'failed', failure: 'restic_failed', message: 'Fatal: wrong password' }),
      ),
    ).toBe('restic a échoué : Fatal: wrong password')
    expect(runFailureText(run({ state: 'failed', failure: 'not_allowed' }))).toContain('polkit')
    expect(runFailureText(run())).toBeNull()
  })

  it('formats durations and statistics', () => {
    expect(formatDuration(0.2)).toBe('1 s')
    expect(formatDuration(90)).toBe('1 min 30 s')
    expect(formatDuration(3720)).toBe('1 h 2 min')
    expect(describeStats(null)).toBe('—')
    expect(
      describeStats({
        filesNew: 3,
        filesChanged: 1,
        filesUnmodified: 0,
        dirsNew: 0,
        dirsChanged: 0,
        dirsUnmodified: 0,
        dataAdded: 2048,
        totalFilesProcessed: 4,
        totalBytesProcessed: 0,
        durationSeconds: 1,
      }),
    ).toContain('3 nouveau(x)')
  })

  it('sums up the health of the backups', () => {
    const base: BackupSummary = {
      configured: true,
      initialized: true,
      running: null,
      lastRun: run(),
      lastSuccess: run(),
      lastCheck: null,
      nextRunAt: null,
    }
    expect(backupHealth({ ...base, configured: false }).label).toBe('Non configurées')
    expect(backupHealth({ ...base, initialized: false }).tone).toBe('warning')
    expect(backupHealth(base).tone).toBe('success')
    expect(backupHealth({ ...base, lastRun: run({ state: 'failed' }) }).tone).toBe('danger')
    expect(backupHealth({ ...base, running: run({ state: 'running' }) }).tone).toBe('info')
    expect(backupHealth({ ...base, lastRun: null, lastSuccess: null }).label).toBe(
      'Aucune sauvegarde',
    )
  })

  it('converts the configuration to the form and back, keeping the stored keys', () => {
    const values = configToValues(config)
    expect(values).toMatchObject({ type: 's3', sources: '/etc\n/srv', username: '', time: '04:30' })
    const input = valuesToInput({ ...values, sources: '/etc\n\n /srv \n' })
    expect(input.sources).toEqual(['/etc', '/srv'])
    expect(input.username).toBeNull()
    expect(input.secret).toBeNull()
    expect(valuesToInput({ ...values, username: 'new-key' }).username).toBe('new-key')
    const local = valuesToInput({ ...values, type: 'local', location: '/mnt/backup' })
    expect(local.repository).toEqual({ type: 'local', location: '/mnt/backup' })
    expect(local.username).toBe('')
    expect(valuesToInput({ ...values, type: 'none' }).repository).toBeNull()
  })

  it('names the keys of each repository type', () => {
    expect(credentialLabels('local')).toBeNull()
    expect(credentialLabels('s3')?.secret).toBe('Clé secrète')
  })

  it('labels and describes the maintenance operations', () => {
    expect(runLabel(run({ operation: 'forget-prune', origin: 'schedule' }))).toBe(
      'Rétention planifiée',
    )
    expect(runLabel(run({ operation: 'check', origin: 'manual' }))).toBe('Vérification du dépôt')
    expect(
      runLabel(run({ operation: 'restore', origin: 'manual', details: { dryRun: true } })),
    ).toBe('Restauration simulée')
    expect(
      describeDetails(
        run({
          operation: 'restore',
          details: { filesRestored: 3, target: '/var/lib/homelab-backup/restore/x' },
        }),
      ),
    ).toBe('3 fichier(s) restauré(s) dans /var/lib/homelab-backup/restore/x')
    expect(
      describeDetails(run({ operation: 'forget-prune', details: { snapshotsRemoved: 2 } })),
    ).toBe('2 instantané(s) supprimé(s)')
    expect(describeDetails(run({ operation: 'check', details: { subsetPercent: 5 } }))).toBe(
      '5 % des données relues sans erreur',
    )
    expect(
      describeDetails(run({ operation: 'check', state: 'failed', details: { subsetPercent: 5 } })),
    ).toBeNull()
  })

  it('converts the retention and the check', () => {
    const values = configToValues(config)
    expect(values).toMatchObject({ retentionEnabled: false, keepDaily: 7, checkSubsetPercent: 5 })
    expect(valuesToInput({ ...values, retentionEnabled: true, keepYearly: 2 }).retention).toEqual({
      enabled: true,
      keepDaily: 7,
      keepWeekly: 4,
      keepMonthly: 6,
      keepYearly: 2,
    })
  })
})
