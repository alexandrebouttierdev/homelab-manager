import { describe, expect, it } from 'vitest'
import type { ContainerAction, ContainerLogLine } from './api/containers'
import {
  containerActionChoices,
  containerActionQuestion,
  describeContainerAction,
  filterLogLines,
} from './containerActions'

const action = (overrides: Partial<ContainerAction>): ContainerAction => ({
  containerId: 'c',
  name: 'web',
  action: 'restart',
  state: 'running',
  failure: null,
  requestedBy: 'olivia',
  requestedAt: '2026-11-25T10:00:00Z',
  finishedAt: null,
  ...overrides,
})

describe('containerActionChoices', () => {
  it('fits the state of the container', () => {
    expect(containerActionChoices('running', false, true)).toEqual(['restart', 'stop'])
    expect(containerActionChoices('exited', false, true)).toEqual(['start'])
    expect(containerActionChoices('created', false, true)).toEqual(['start'])
  })

  it('offers nothing while disabled or once removed', () => {
    expect(containerActionChoices('running', false, false)).toEqual([])
    expect(containerActionChoices('exited', true, true)).toEqual([])
  })
})

describe('containerActionQuestion', () => {
  it('names the container and the consequence', () => {
    expect(containerActionQuestion('stop', 'web')).toMatch(
      /^Arrêter le conteneur web \?.*indisponibles/,
    )
    expect(containerActionQuestion('restart', 'web')).toMatch(/brièvement interrompus/)
    expect(containerActionQuestion('start', 'web')).toBe('Démarrer le conteneur web ?')
  })
})

describe('describeContainerAction', () => {
  it('tells where the action stands', () => {
    expect(describeContainerAction(action({}))).toEqual({
      tone: 'info',
      message: 'Redémarrage de web en cours…',
    })
    expect(describeContainerAction(action({ state: 'succeeded' })).message).toBe('web redémarré.')
    expect(
      describeContainerAction(action({ state: 'failed', failure: 'not_allowed' })).message,
    ).toMatch(/MONOOPS_DOCKER_PROXY_CONTAINER_ACTIONS/)
  })
})

describe('filterLogLines', () => {
  const lines: ContainerLogLine[] = [
    { stream: 'stdout', time: null, message: 'GET /health 200', cut: false },
    { stream: 'stderr', time: null, message: 'Erreur de connexion', cut: false },
  ]
  it('filters by stream and text, case ignored', () => {
    expect(filterLogLines(lines, 'all', '')).toHaveLength(2)
    expect(filterLogLines(lines, 'stderr', '')).toEqual([lines[1]])
    expect(filterLogLines(lines, 'all', 'erreur')).toEqual([lines[1]])
    expect(filterLogLines(lines, 'stdout', 'erreur')).toEqual([])
  })
})
