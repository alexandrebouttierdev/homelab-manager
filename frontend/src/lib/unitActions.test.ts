import { describe, expect, it } from 'vitest'
import type { SystemUnit, UnitAction } from './api/services'
import { describeUnitAction, unitActionChoices, unitActionQuestion } from './unitActions'

function unit(name: string, activeState: string, overrides: Partial<SystemUnit> = {}): SystemUnit {
  return {
    name,
    description: '',
    type: name.endsWith('.timer') ? 'timer' : 'service',
    loadState: 'loaded',
    activeState,
    subState: 'running',
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

function action(overrides: Partial<UnitAction> = {}): UnitAction {
  return {
    unit: 'nginx.service',
    action: 'restart',
    state: 'running',
    failure: null,
    requestedBy: 'olivia',
    requestedAt: '2026-11-24T10:00:00Z',
    finishedAt: null,
    final: null,
    ...overrides,
  }
}

describe('unit actions', () => {
  it('offers the verbs that fit the state, on units of the allow list only', () => {
    const verbs = (u: SystemUnit, list: string[]) =>
      unitActionChoices(u, list).map((choice) => `${choice.verb} ${choice.unit}`)
    expect(verbs(unit('nginx.service', 'active'), ['nginx.service'])).toEqual([
      'restart nginx.service',
      'reload nginx.service',
      'stop nginx.service',
    ])
    expect(verbs(unit('nginx.service', 'failed'), ['nginx.service'])).toEqual([
      'start nginx.service',
      'restart nginx.service',
    ])
    expect(verbs(unit('ssh.service', 'active'), ['nginx.service'])).toEqual([])
    expect(verbs(unit('ssh.service', 'active'), [])).toEqual([])
  })

  it('runs the service of a timer when that service is in the allow list', () => {
    const timer = unit('backup.timer', 'active', { triggers: ['backup.service'] })
    expect(unitActionChoices(timer, ['backup.service'])).toEqual([
      { unit: 'backup.service', verb: 'start', label: 'Exécuter' },
    ])
    expect(unitActionChoices(timer, ['backup.timer']).map((choice) => choice.verb)).toEqual([
      'restart',
      'reload',
      'stop',
    ])
    expect(unitActionChoices(timer, [])).toEqual([])
  })

  it('names the target in the confirmation', () => {
    expect(unitActionQuestion({ unit: 'nginx.service', verb: 'stop', label: 'Arrêter' })).toBe(
      "Arrêter nginx.service ? Le service sera indisponible jusqu'à son prochain démarrage.",
    )
    expect(
      unitActionQuestion(
        { unit: 'backup.service', verb: 'start', label: 'Exécuter' },
        'backup.timer',
      ),
    ).toBe('Exécuter maintenant backup.service, déclenché par backup.timer ?')
  })

  it('describes each outcome', () => {
    expect(describeUnitAction(action())).toEqual({
      tone: 'info',
      message: 'Redémarrage de nginx.service en cours…',
    })
    expect(
      describeUnitAction(
        action({ state: 'succeeded', final: { activeState: 'active', subState: 'running' } }),
      ),
    ).toEqual({ tone: 'success', message: 'nginx.service redémarrée (état : active).' })
    const refused = describeUnitAction(action({ state: 'failed', failure: 'not_allowed' }))
    expect(refused.tone).toBe('danger')
    expect(refused.message).toContain('monoops polkit-rule')
    expect(describeUnitAction(action({ state: 'failed', failure: 'timeout' })).message).toContain(
      '90 secondes',
    )
  })
})
