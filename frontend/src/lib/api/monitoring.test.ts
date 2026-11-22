import { afterEach, describe, expect, it, vi } from 'vitest'
import { getHostMetrics, stepForRange } from './monitoring'

const hour = 60 * 60 * 1000

describe('stepForRange', () => {
  it('follows the recommended steps', () => {
    expect(stepForRange(hour)).toBe('raw')
    expect(stepForRange(6 * hour)).toBe('1m')
    expect(stepForRange(24 * hour)).toBe('5m')
    expect(stepForRange(7 * 24 * hour)).toBe('1h')
  })

  it('gives the same step to a range a few milliseconds longer', () => {
    expect(stepForRange(hour + 5)).toBe('raw')
    expect(stepForRange(24 * hour + 5)).toBe('5m')
  })
})

describe('getHostMetrics', () => {
  afterEach(() => vi.unstubAllGlobals())

  function respond(...bodies: Array<{ status: number; body: unknown }>) {
    const fetchMock = vi.fn()
    for (const { status, body } of bodies) {
      fetchMock.mockResolvedValueOnce(new Response(JSON.stringify(body), { status }))
    }
    vi.stubGlobal('fetch', fetchMock)
    return fetchMock
  }

  function requestedStep(fetchMock: ReturnType<typeof vi.fn>, call: number) {
    return new URL(String(fetchMock.mock.calls[call][0]), 'http://localhost').searchParams.get(
      'step',
    )
  }

  it('sends the label filter and the step of the range', async () => {
    const fetchMock = respond({ status: 200, body: { samples: [] } })
    const to = new Date('2026-11-22T12:00:00Z')
    await getHostMetrics('cpu.percent', new Date(to.getTime() - 24 * hour), {
      to,
      labels: { core: 'all' },
    })
    const url = new URL(String(fetchMock.mock.calls[0][0]), 'http://localhost')
    expect(url.pathname).toBe('/api/v1/host/metrics/cpu.percent')
    expect(url.searchParams.get('step')).toBe('5m')
    expect(url.searchParams.getAll('label')).toEqual(['core:all'])
  })

  it('moves to a coarser step on TOO_MANY_POINTS', async () => {
    const fetchMock = respond(
      { status: 400, body: { error: { code: 'TOO_MANY_POINTS' } } },
      { status: 200, body: { samples: [{ name: 'cpu.percent', value: 1 }] } },
    )
    const to = new Date('2026-11-22T12:00:00Z')
    const samples = await getHostMetrics('cpu.percent', new Date(to.getTime() - 24 * hour), { to })
    expect(samples).toHaveLength(1)
    expect(requestedStep(fetchMock, 0)).toBe('5m')
    expect(requestedStep(fetchMock, 1)).toBe('1h')
  })

  it('moves to an aggregated step when a raw answer fills the limit', async () => {
    const full = Array.from({ length: 2000 }, () => ({ name: 'cpu.percent', value: 1 }))
    const fetchMock = respond(
      { status: 200, body: { samples: full } },
      { status: 200, body: { samples: [] } },
    )
    const to = new Date('2026-11-22T12:00:00Z')
    await getHostMetrics('cpu.percent', new Date(to.getTime() - hour), { to })
    expect(requestedStep(fetchMock, 0)).toBe('raw')
    expect(requestedStep(fetchMock, 1)).toBe('1m')
  })
})
