import { describe, expect, it } from 'vitest'
import type { SystemUnit } from './api/services'
import {
  filterUnits,
  formatCpuTime,
  formatUnitLimit,
  oldestFirst,
  unitFileStateLabel,
  unitStateCounts,
  unitStateLabel,
  unitStateTone,
  unitsStatusInfo,
} from './services'

function unit(name: string, activeState: string, overrides: Partial<SystemUnit> = {}): SystemUnit {
  return {
    name,
    description: `Unité ${name}`,
    type: name.endsWith('.timer') ? 'timer' : name.endsWith('.socket') ? 'socket' : 'service',
    loadState: 'loaded',
    activeState,
    subState: activeState === 'active' ? 'running' : 'dead',
    unitFileState: 'enabled',
    stateChangedAt: null,
    mainPid: null,
    memoryBytes: null,
    cpuNanoseconds: null,
    result: null,
    nextElapse: null,
    lastTrigger: null,
    triggers: [],
    ...overrides,
  }
}

const units = [
  unit('web.service', 'active'),
  unit('backup.timer', 'active'),
  unit('zzz.service', 'failed'),
  unit('docker.socket', 'active'),
  unit('old.service', 'inactive', { description: 'Ancien service de sauvegarde' }),
  unit('aaa.service', 'failed'),
]

describe('filterUnits', () => {
  it('puts failed units first, then sorts by name', () => {
    expect(
      filterUnits(units, { type: 'all', state: 'all', search: '' }).map((u) => u.name),
    ).toEqual([
      'aaa.service',
      'zzz.service',
      'backup.timer',
      'docker.socket',
      'old.service',
      'web.service',
    ])
  })

  it('filters by type, state and name or description', () => {
    expect(
      filterUnits(units, { type: 'timer', state: 'all', search: '' }).map((u) => u.name),
    ).toEqual(['backup.timer'])
    expect(filterUnits(units, { type: 'service', state: 'failed', search: '' })).toHaveLength(2)
    expect(
      filterUnits(units, { type: 'all', state: 'other', search: '' }).map((u) => u.name),
    ).toEqual(['old.service'])
    expect(
      filterUnits(units, { type: 'all', state: 'all', search: '  SAUVEGARDE ' }).map((u) => u.name),
    ).toEqual(['old.service'])
    expect(filterUnits(units, { type: 'all', state: 'active', search: 'web' })).toHaveLength(1)
  })

  it('counts each state of the filter', () => {
    expect(unitStateCounts(units)).toEqual({ all: 6, active: 3, failed: 2, other: 1 })
  })
})

describe('labels', () => {
  it('names the states and their tones', () => {
    expect(unitStateLabel('active')).toBe('Actif')
    expect(unitStateLabel('failed')).toBe('En échec')
    expect(unitStateLabel('reloading')).toBe('Rechargement')
    expect(unitStateLabel('custom')).toBe('custom')
    expect(unitStateTone('active')).toBe('success')
    expect(unitStateTone('failed')).toBe('danger')
    expect(unitStateTone('activating')).toBe('warning')
    expect(unitStateTone('inactive')).toBe('neutral')
  })

  it('names the unit file states, unknown as a dash', () => {
    expect(unitFileStateLabel('enabled')).toBe('Activé')
    expect(unitFileStateLabel('static')).toBe('Statique')
    expect(unitFileStateLabel(null)).toBe('—')
  })

  it('tells the collection status', () => {
    expect(unitsStatusInfo('ok').tone).toBe('success')
    expect(unitsStatusInfo('unavailable').label).toMatch(/indisponible/)
    expect(unitsStatusInfo('error').tone).toBe('danger')
    expect(unitsStatusInfo('pending').tone).toBe('neutral')
  })
})

describe('formatting', () => {
  it('formats the CPU time, absent as a dash and never 0', () => {
    expect(formatCpuTime(null)).toBe('—')
    expect(formatCpuTime(0)).toBe('0 s')
    expect(formatCpuTime(1_500_000_000)).toBe('1,5 s')
    expect(formatCpuTime(20_000_000)).toBe('20 ms')
    expect(formatCpuTime(1_000)).toBe('1 ms')
    expect(formatCpuTime(125_000_000_000)).toBe('2 min 5 s')
    expect(formatCpuTime(7_260_000_000_000)).toBe('2 h 01 min')
  })

  it('tells an unlimited limit from an unknown one', () => {
    expect(formatUnitLimit({ value: null, unlimited: true })).toBe('illimité')
    expect(formatUnitLimit({ value: null, unlimited: false })).toBe('inconnu')
    expect(formatUnitLimit({ value: 1024, unlimited: false })).toBe('1\u202f024')
    expect(formatUnitLimit({ value: 536_870_912, unlimited: false }, 'bytes')).toBe('512 Mo')
  })

  it('lists journal entries oldest first without changing the input', () => {
    const newestFirst = [{ cursor: 'b' }, { cursor: 'a' }]
    expect(oldestFirst(newestFirst).map((entry) => entry.cursor)).toEqual(['a', 'b'])
    expect(newestFirst[0].cursor).toBe('b')
  })
})
