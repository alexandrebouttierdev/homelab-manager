import { describe, expect, it } from 'vitest'
import { backupConfigSchema, type BackupConfigValues } from './backupConfig'

const valid: BackupConfigValues = {
  type: 'local',
  location: '/mnt/backup',
  username: '',
  secret: '',
  sources: '/etc\n/var/lib/docker/volumes',
  excludes: '*.tmp\n**/node_modules',
  excludeCaches: true,
  excludeIfPresent: '.nobackup',
  scheduleEnabled: true,
  frequency: 'daily',
  weekday: 0,
  time: '03:00',
  retentionEnabled: false,
  keepDaily: 7,
  keepWeekly: 4,
  keepMonthly: 6,
  keepYearly: 0,
  checkEnabled: true,
  checkSubsetPercent: 5,
}

function errors(values: Partial<BackupConfigValues>) {
  const result = backupConfigSchema.safeParse({ ...valid, ...values })
  return result.success ? [] : result.error.issues.map((issue) => issue.path.join('.'))
}

describe('backupConfigSchema', () => {
  it('accepts a valid configuration', () => {
    expect(errors({})).toEqual([])
    expect(errors({ type: 's3', location: 'https://s3.example.com/bucket/prefix' })).toEqual([])
    expect(errors({ type: 'none', location: '' })).toEqual([])
  })

  it('refuses invalid locations, sources, exclusions and times', () => {
    expect(errors({ location: 'relative' })).toContain('location')
    expect(errors({ location: '/mnt/../etc' })).toContain('location')
    expect(errors({ type: 'rest', location: 'http://backup.lan' })).toContain('location')
    expect(errors({ type: 's3', location: 'https://s3.example.com' })).toContain('location')
    expect(errors({ sources: '/etc\netc' })).toContain('sources')
    expect(errors({ sources: '/etc\n/etc' })).toContain('sources')
    expect(errors({ excludes: '#comment' })).toContain('excludes')
    expect(errors({ excludeIfPresent: 'a/b' })).toContain('excludeIfPresent')
    expect(errors({ time: '25:00' })).toContain('time')
  })

  it('checks the retention and the check', () => {
    expect(
      errors({
        retentionEnabled: true,
        keepDaily: 0,
        keepWeekly: 0,
        keepMonthly: 0,
        keepYearly: 0,
      }),
    ).toContain('keepDaily')
    expect(
      errors({
        retentionEnabled: false,
        keepDaily: 0,
        keepWeekly: 0,
        keepMonthly: 0,
        keepYearly: 0,
      }),
    ).toEqual([])
    expect(errors({ keepDaily: -1 })).toContain('keepDaily')
    expect(errors({ checkSubsetPercent: 0 })).toContain('checkSubsetPercent')
  })
})
