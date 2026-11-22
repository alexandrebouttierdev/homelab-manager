import { describe, expect, it } from 'vitest'
import { matchesContainerSearch, matchesStateFilter, sortContainers } from './containers'

const base = { dockerId: 'abc123', status: 'Up', networks: ['bridge'], ports: [], restartCount: 0 }
const web = { ...base, name: 'web', image: 'nginx:stable', state: 'running', uptimeSeconds: 50 }
const db = {
  ...base,
  name: 'db',
  image: 'postgres:17',
  state: 'exited',
  uptimeSeconds: null,
  restartCount: 3,
}
const cache = { ...base, name: 'cache', image: 'redis', state: 'restarting', uptimeSeconds: 10 }

describe('matchesContainerSearch', () => {
  it('searches name, image, networks and ports case-insensitively', () => {
    expect(matchesContainerSearch(web, 'NGINX')).toBe(true)
    expect(matchesContainerSearch(web, 'bridge')).toBe(true)
    expect(matchesContainerSearch(web, 'postgres')).toBe(false)
    expect(matchesContainerSearch(web, '')).toBe(true)
  })
})

describe('matchesStateFilter', () => {
  it('maps filters to Docker states', () => {
    expect(matchesStateFilter('running', 'running')).toBe(true)
    expect(matchesStateFilter('exited', 'stopped')).toBe(true)
    expect(matchesStateFilter('restarting', 'error')).toBe(true)
    expect(matchesStateFilter('running', 'stopped')).toBe(false)
    expect(matchesStateFilter('dead', 'all')).toBe(true)
  })
})

describe('sortContainers', () => {
  it('sorts without mutating the input', () => {
    const input = [web, db, cache]
    expect(sortContainers(input, 'name').map((c) => c.name)).toEqual(['cache', 'db', 'web'])
    expect(sortContainers(input, 'uptime').map((c) => c.name)).toEqual(['web', 'cache', 'db'])
    expect(sortContainers(input, 'restarts').map((c) => c.name)).toEqual(['db', 'cache', 'web'])
    expect(input.map((c) => c.name)).toEqual(['web', 'db', 'cache'])
  })
})
