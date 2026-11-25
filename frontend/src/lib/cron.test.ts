import { describe, expect, it } from 'vitest'
import type { CronDirectory, CronEntry, CronTimer } from './api/cron'
import { directorySchedule, filterCronEntries, sortTimers, timerResult } from './cron'

function entry(overrides: Partial<CronEntry> = {}): CronEntry {
  return {
    source: '/etc/crontab',
    line: 7,
    expression: '25 6 * * *',
    user: 'root',
    command: 'run-parts --report /etc/cron.daily',
    valid: true,
    error: null,
    description: 'Tous les jours à 06:25',
    nextRuns: [],
    lastRun: null,
    ...overrides,
  }
}

function timer(
  name: string,
  nextElapse: string | null,
  serviceResult: string | null = null,
): CronTimer {
  return {
    name,
    description: '',
    activeState: 'active',
    nextElapse,
    lastTrigger: null,
    service: null,
    serviceResult,
  }
}

describe('cron', () => {
  it('tells how a run-parts directory is started', () => {
    const daily: CronDirectory = {
      period: 'daily',
      path: '/etc/cron.daily',
      scripts: [],
      scheduledBy: entry(),
    }
    expect(directorySchedule(daily, false)).toBe('Tous les jours à 06:25')
    expect(directorySchedule(daily, true)).toBe('Par anacron (à défaut : tous les jours à 06:25)')
    expect(directorySchedule({ ...daily, scheduledBy: null }, false)).toContain('Aucune ligne')
    expect(directorySchedule({ ...daily, period: 'hourly' }, true)).toBe('Tous les jours à 06:25')
  })

  it('filters the entries', () => {
    const entries = [
      entry(),
      entry({ user: 'alice', command: '/opt/backup.sh', source: '/etc/cron.d/backup' }),
    ]
    expect(filterCronEntries(entries, 'BACKUP')).toHaveLength(1)
    expect(filterCronEntries(entries, 'tous les jours')).toHaveLength(2)
    expect(filterCronEntries(entries, '  ')).toHaveLength(2)
  })

  it('labels the result of a timer and sorts timers by next run', () => {
    expect(timerResult(timer('a', null, 'success')).tone).toBe('success')
    expect(timerResult(timer('a', null, 'exit-code')).tone).toBe('danger')
    expect(timerResult(timer('a', null)).label).toBe('Inconnu')
    const sorted = sortTimers([
      timer('c', null),
      timer('b', '2026-11-25T00:00:00Z'),
      timer('a', '2026-11-24T12:00:00Z'),
    ])
    expect(sorted.map((t) => t.name)).toEqual(['a', 'b', 'c'])
  })
})
