import { afterEach, describe, expect, it, vi } from 'vitest'
import { getUnit, listUnits } from './services'

function mockFetch(body: unknown, status = 200) {
  const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status }))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

describe('system units API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('reads the inventory', async () => {
    const inventory = {
      status: 'unavailable',
      collectedAt: null,
      counts: { total: 0, active: 0, failed: 0, activeTimers: 0 },
      units: [],
    }
    const fetchMock = mockFetch(inventory)
    await expect(listUnits()).resolves.toEqual(inventory)
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/system/units')
  })

  it('encodes the unit name of a detail', async () => {
    const fetchMock = mockFetch({
      unit: {},
      definition: {},
      journal: { available: false, entries: [] },
    })
    await getUnit('getty@tty1.service')
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/system/units/getty%40tty1.service')
  })

  it('explains an unreachable systemd', async () => {
    mockFetch({ error: { code: 'SYSTEMD_UNAVAILABLE', message: 'no bus' } }, 503)
    await expect(getUnit('ssh.service')).rejects.toThrow(/systemd/)
  })
})
