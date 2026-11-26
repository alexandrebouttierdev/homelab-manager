import { describe, expect, it } from 'vitest'
import type { BackupConfig, BackupRun, BackupSummary } from './api/backups'
import {
  archiveUrl,
  backupHealth,
  inPlaceIssue,
  configToValues,
  copyToValues,
  copyValuesToInput,
  describeDetails,
  diffModifierInfo,
  orderForDiff,
  credentialLabels,
  describeSchedule,
  describeStats,
  formatDuration,
  locationHint,
  repositoryLabel,
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
  sqliteDatabases: [],
  schedule: { enabled: true, frequency: 'weekly', weekday: 0, time: '04:30' },
  retention: { enabled: false, keepDaily: 7, keepWeekly: 4, keepMonthly: 6, keepYearly: 0 },
  check: { enabled: true, subsetPercent: 5 },
  updatedAt: null,
  updatedBy: null,
  copy: null,
  sshKey: null,
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
      lastCopy: null,
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

  it('keeps the host key of an SFTP repository only', () => {
    const sftp: BackupConfig = {
      ...config,
      repository: {
        type: 'sftp',
        location: 'sftp://backup@nas.lan/srv/restic',
        hostKey: 'ssh-ed25519 AAAA',
        hostKeyFingerprint: 'SHA256:abc',
      },
    }
    const values = configToValues(sftp)
    expect(values).toMatchObject({ type: 'sftp', hostKey: 'ssh-ed25519 AAAA' })
    expect(valuesToInput({ ...values, hostKey: ' nas.lan ssh-ed25519 BBBB\n' }).repository).toEqual(
      {
        type: 'sftp',
        location: 'sftp://backup@nas.lan/srv/restic',
        hostKey: 'nas.lan ssh-ed25519 BBBB',
      },
    )
    expect(valuesToInput({ ...values, type: 'local', location: '/mnt/b' }).repository).toEqual({
      type: 'local',
      location: '/mnt/b',
    })
    expect(valuesToInput(values).username).toBe('')
    expect(
      copyValuesToInput({
        ...copyToValues(null),
        type: 'sftp',
        location: 'sftp://c@h/srv/c',
        hostKey: 'k',
      }),
    ).toMatchObject({
      repository: { type: 'sftp', location: 'sftp://c@h/srv/c', hostKey: 'k' },
      username: '',
    })
    expect(credentialLabels('sftp')).toBeNull()
    expect(locationHint('sftp')).toContain('sftp://utilisateur@hôte')
    expect(repositoryLabel('sftp')).toBe('SFTP')
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
          details: { filesRestored: 3, target: '/var/lib/monoops-backup/restore/x' },
        }),
      ),
    ).toBe('3 fichier(s) restauré(s) dans /var/lib/monoops-backup/restore/x')
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

  it('labels and describes a diff and a copy', () => {
    const diff = {
      from: '3a253ad8aaaa',
      to: '9f2c11b0bbbb',
      changedFiles: 2,
      added: { files: 3, dirs: 0, others: 0, bytes: 2048 },
      removed: { files: 1, dirs: 0, others: 0, bytes: 10 },
      changes: [],
      totalChanges: 6,
      truncated: false,
    }
    expect(runLabel(run({ operation: 'diff', origin: 'manual' }))).toBe("Comparaison d'instantanés")
    expect(describeDetails(run({ operation: 'diff', details: { diff } }))).toBe(
      '2 modifié(s), 3 ajouté(s) (2 Ko), 1 supprimé(s) (10 o)',
    )
    expect(runLabel(run({ operation: 'copy', origin: 'schedule' }))).toBe(
      'Copie vers le dépôt de copie planifiée',
    )
    const copy = {
      repository: { type: 'local' as const, location: '/mnt/copy' },
      initialized: true,
      snapshotsCopied: 2,
      snapshotsInTarget: 5,
      snapshotsRemoved: 1,
    }
    expect(describeDetails(run({ operation: 'copy', details: { copy } }))).toBe(
      'Dépôt de copie créé · 2 instantané(s) copié(s) · 1 supprimé(s) par la rétention · 5 dans la copie',
    )
    expect(diffModifierInfo('+')).toEqual({ label: 'Ajouté', tone: 'success' })
    expect(diffModifierInfo('?').label).toBe('Contenu altéré')
  })

  it('orders two snapshots for a diff, older first', () => {
    const older = { id: 'a', time: '2026-11-23T03:00:00Z' }
    const newer = { id: 'b', time: '2026-11-24T03:00:00Z' }
    expect(orderForDiff(newer, older)).toEqual([older, newer])
    expect(orderForDiff(older, newer)).toEqual([older, newer])
  })

  it('converts the copy repository to the form and back', () => {
    expect(copyToValues(null)).toEqual({
      type: 'local',
      location: '',
      hostKey: '',
      username: '',
      secret: '',
      afterBackup: true,
    })
    const stored = {
      repository: { type: 'rest' as const, location: 'https://nas.lan:8000/copy' },
      initialized: true,
      initializedAt: null,
      credentials: { password: true, username: true, secret: true },
      afterBackup: false,
    }
    const values = copyToValues(stored)
    expect(values).toMatchObject({ type: 'rest', afterBackup: false, username: '' })
    expect(copyValuesToInput(values)).toEqual({
      repository: { type: 'rest', location: 'https://nas.lan:8000/copy' },
      username: null,
      secret: null,
      afterBackup: false,
    })
    expect(copyValuesToInput({ ...values, type: 'local', location: ' /mnt/copy ' })).toMatchObject({
      repository: { type: 'local', location: '/mnt/copy' },
      username: '',
      secret: '',
    })
  })
})

describe('in-place restores and archives', () => {
  it('refuses the protected trees', () => {
    expect(inPlaceIssue([])).toMatch(/jamais tout/)
    expect(inPlaceIssue(['/'])).toMatch(/machine entière/)
    expect(inPlaceIssue(['/etc/nginx'])).toMatch(/\/etc/)
    expect(inPlaceIssue(['/var'])).toMatch(/monoops/)
    expect(inPlaceIssue(['/srv/app', '/var/lib/docker/volumes/web'])).toBeNull()
  })

  it('serves an archive for 24 hours', () => {
    const run = {
      id: 'r',
      operation: 'dump',
      origin: 'manual',
      requestedBy: 'admin',
      state: 'succeeded',
      failure: null,
      message: null,
      warning: false,
      snapshotId: null,
      stats: null,
      details: { archivePath: '/srv', archiveBytes: 1024 },
      startedAt: '2026-11-25T10:00:00Z',
      finishedAt: '2026-11-25T10:01:00Z',
    } as BackupRun
    expect(archiveUrl(run, new Date('2026-11-25T12:00:00Z'))).toBe('/api/v1/backups/runs/r/archive')
    expect(archiveUrl(run, new Date('2026-11-26T11:00:00Z'))).toBeNull()
    expect(archiveUrl({ ...run, state: 'failed' }, new Date('2026-11-25T12:00:00Z'))).toBeNull()
    expect(describeDetails(run)).toBe('Archive de /srv (1 Ko)')
    expect(
      runLabel({ ...run, operation: 'restore-in-place', details: { dryRun: true, inPlace: true } }),
    ).toBe("Restauration à l'emplacement d'origine simulée")
  })
})
