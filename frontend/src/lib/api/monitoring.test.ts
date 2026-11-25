import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  getHostMetrics,
  listContainers,
  listEventsPage,
  maxContainerPages,
  stepForRange,
} from './monitoring'

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

describe('cursor pagination', () => {
  afterEach(() => vi.unstubAllGlobals())

  function container(id: string) {
    return { id, name: id }
  }

  it('follows nextCursor through the container inventory', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ containers: [container('a')], nextCursor: 'c/1+' })),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ containers: [container('b')], nextCursor: null })),
      )
    vi.stubGlobal('fetch', fetchMock)
    const containers = await listContainers(true)
    expect(containers.map((item) => item.id)).toEqual(['a', 'b'])
    expect(String(fetchMock.mock.calls[0][0])).toBe('/api/v1/containers?includeRemoved=true')
    const second = new URL(String(fetchMock.mock.calls[1][0]), 'http://localhost')
    expect(second.searchParams.get('cursor')).toBe('c/1+')
    expect(second.searchParams.get('includeRemoved')).toBe('true')
  })

  it('stops after maxContainerPages pages', async () => {
    const fetchMock = vi.fn(
      async () =>
        new Response(JSON.stringify({ containers: [container('x')], nextCursor: 'more' })),
    )
    vi.stubGlobal('fetch', fetchMock)
    const containers = await listContainers()
    expect(fetchMock).toHaveBeenCalledTimes(maxContainerPages)
    expect(containers).toHaveLength(maxContainerPages)
  })

  it('sends the cursor of an event page', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify({ events: [], nextCursor: null })))
    vi.stubGlobal('fetch', fetchMock)
    await listEventsPage({
      from: new Date('2026-11-25T07:00:00Z'),
      to: new Date('2026-11-25T09:00:00Z'),
      limit: 500,
      cursor: 'next',
    })
    const url = new URL(String(fetchMock.mock.calls[0][0]), 'http://localhost')
    expect(url.pathname).toBe('/api/v1/events')
    expect(url.searchParams.get('cursor')).toBe('next')
    expect(url.searchParams.get('limit')).toBe('500')
    expect(url.searchParams.get('from')).toBe('2026-11-25T07:00:00.000Z')
  })
})
