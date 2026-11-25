import { afterEach, describe, expect, it, vi } from 'vitest'
import { getCron, inspectCron } from './cron'

function mockFetch(body: unknown, status = 200) {
  const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status }))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

describe('cron API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('reads the scheduled tasks and encodes an expression', async () => {
    let fetchMock = mockFetch({ entries: [] })
    await getCron()
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/cron')
    fetchMock = mockFetch({ description: 'x', nextRuns: [] })
    await inspectCron('*/5 * * * *')
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/cron/inspect?expression=*%2F5+*+*+*+*')
  })

  it('explains an invalid expression', async () => {
    mockFetch({ error: { code: 'CRON_EXPRESSION_INVALID', message: 'bad' } }, 400)
    await expect(inspectCron('61 * * * *')).rejects.toThrow(/5 champs/)
  })
})
