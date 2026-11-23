import { describe, expect, it } from 'vitest'
import type { JournalEntry, JournalSummary } from './api/journal'
import {
  bootLabel,
  displayMessage,
  fieldsJson,
  journalPeriodStart,
  maxLiveEntries,
  mergeNewer,
  priorityLabel,
  priorityTone,
  summaryChartOption,
  summaryTotals,
  toJournalRow,
  visibleEntries,
} from './journal'

function entry(n: number, overrides: Partial<JournalEntry> = {}): JournalEntry {
  return {
    cursor: `c${n}`,
    time: '2026-11-23T09:30:12Z',
    priority: 6,
    unit: '',
    identifier: 'sshd',
    pid: '',
    hostname: 'homelab',
    bootId: 'b',
    message: `message ${n}`,
    truncated: false,
    ...overrides,
  }
}

describe('journal view helpers', () => {
  it('labels and colours the priorities', () => {
    expect(priorityLabel(3)).toBe('Erreur')
    expect(priorityLabel(null)).toBe('Sans priorité')
    expect([0, 3, 4, 5, 6, 7, null].map(priorityTone)).toEqual([
      'danger',
      'danger',
      'warning',
      'info',
      'neutral',
      'neutral',
      'neutral',
    ])
  })

  it('removes terminal escape sequences and control characters', () => {
    expect(displayMessage('\u001b[1;31mred\u001b[0m text')).toBe('red text')
    expect(displayMessage('a\u0007b\u0000c\td\ne\u007f')).toBe('abc\td\ne')
    expect(displayMessage('\u001b(B plain')).toBe('B plain')
    expect(displayMessage('plain é')).toBe('plain é')
  })

  it('builds a row with the unit, else the identifier', () => {
    const row = toJournalRow(entry(1, { unit: 'ssh.service', pid: '42', priority: 3 }))
    expect(row).toMatchObject({
      source: 'ssh.service',
      identifier: 'sshd[42]',
      priorityTone: 'danger',
    })
    expect(toJournalRow(entry(2)).source).toBe('sshd')
    expect(toJournalRow(entry(3, { identifier: '' })).source).toBe('—')
  })

  it('merges newer entries above the live ones, once, within the bound', () => {
    expect(
      mergeNewer([entry(2), entry(1)], [entry(4), entry(3), entry(2)]).map((e) => e.cursor),
    ).toEqual(['c4', 'c3', 'c2', 'c1'])
    const many = Array.from({ length: maxLiveEntries + 10 }, (_, i) => entry(i))
    const merged = mergeNewer([], many)
    expect(merged).toHaveLength(maxLiveEntries)
    expect(merged[0].cursor).toBe('c0')
  })

  it('shows live entries above the pages without duplicates', () => {
    expect(
      visibleEntries([entry(5), entry(4)], [[entry(4), entry(3)], [entry(2)]]).map((e) => e.cursor),
    ).toEqual(['c5', 'c4', 'c3', 'c2'])
  })

  it('computes the start of a preset period', () => {
    const now = new Date('2026-11-23T10:00:00Z')
    expect(journalPeriodStart('1h', now)).toBe('2026-11-23T09:00:00.000Z')
    expect(journalPeriodStart('7d', now)).toBe('2026-11-16T10:00:00.000Z')
    expect(journalPeriodStart('all', now)).toBeUndefined()
    expect(journalPeriodStart('custom', now)).toBeUndefined()
  })

  it('labels the boots', () => {
    const boot = {
      index: 0,
      bootId: 'b',
      firstEntry: '2026-11-23T08:00:00Z',
      lastEntry: '2026-11-23T10:00:00Z',
    }
    expect(bootLabel(boot)).toMatch(/^Démarrage actuel \(depuis /)
    expect(bootLabel({ ...boot, index: -1 })).toMatch(/^Démarrage -1 \(.+ → .+\)$/)
  })

  it('writes the raw JSON of the inspector', () => {
    expect(
      JSON.parse(
        fieldsJson([
          { name: 'MESSAGE', value: 'hi', masked: false },
          { name: '_CMDLINE', value: '[masqué]', masked: true },
        ]),
      ),
    ).toEqual({ MESSAGE: 'hi', _CMDLINE: '[masqué]' })
  })

  it('groups the summary by priority', () => {
    const summary: JournalSummary = {
      start: '2026-11-22T10:15:00Z',
      bucketMinutes: 15,
      buckets: [
        { start: '2026-11-22T10:15:00Z', counts: [1, 0, 0, 2, 3, 4, 5, 6] },
        { start: '2026-11-22T10:30:00Z', counts: [0, 0, 0, 0, 1, 0, 0, 1] },
      ],
      total: 24,
      truncated: false,
      diskUsageBytes: 1024,
      fullAccess: true,
      generatedAt: '2026-11-23T10:07:00Z',
    }
    expect(summaryTotals(summary).map((total) => total.total)).toEqual([3, 4, 4, 12])
    const theme = {
      text: 't',
      muted: 'm',
      grid: 'g',
      axis: 'a',
      tooltipBackground: 'b',
      tooltipBorder: 'o',
      tooltipText: 'x',
      cpu: 'c',
      ram: 'r',
      net: 'n',
      accent: 'ac',
      danger: 'd',
      warning: 'w',
    }
    const option = summaryChartOption(summary, theme)
    const series = option.series as { name: string; stack: string; data: number[] }[]
    expect(series.map((item) => item.data)).toEqual([
      [3, 0],
      [3, 1],
      [4, 0],
      [11, 1],
    ])
    expect(new Set(series.map((item) => item.stack)).size).toBe(1)
  })
})
