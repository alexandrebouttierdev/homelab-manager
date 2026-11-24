import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  getFinding,
  getVulnerabilitySummary,
  listFindings,
  requestScan,
  updateScanInterval,
} from './vulnerabilities'

function mockFetch(body: unknown, status = 200) {
  const fetchMock = vi
    .fn()
    .mockImplementation(() => Promise.resolve(new Response(JSON.stringify(body), { status })))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

describe('vulnerabilities API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('builds the findings query from the filters', async () => {
    const fetchMock = mockFetch({ findings: [], total: 0, limit: 50, offset: 100 })
    await listFindings({
      state: 'present',
      severities: ['critical', 'high'],
      fixableOnly: true,
      search: ' openssl ',
      limit: 50,
      offset: 100,
    })
    expect(fetchMock.mock.calls[0][0]).toBe(
      '/api/v1/vulnerabilities?state=present&limit=50&offset=100&severity=critical%2Chigh&fixable=true&q=openssl',
    )
    await listFindings({
      state: 'resolved',
      severities: [],
      fixableOnly: false,
      search: '',
      limit: 100,
      offset: 0,
    })
    expect(fetchMock.mock.calls[1][0]).toBe(
      '/api/v1/vulnerabilities?state=resolved&limit=100&offset=0',
    )
  })

  it('reads the summary and a finding, starts a scan and changes the interval', async () => {
    let fetchMock = mockFetch({ total: 0 })
    await getVulnerabilitySummary()
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/vulnerabilities/summary')

    fetchMock = mockFetch({ finding: { id: 'f/1' } })
    await expect(getFinding('f/1')).resolves.toEqual({ id: 'f/1' })
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/vulnerabilities/f%2F1')

    fetchMock = mockFetch({ scan: { id: 's1', state: 'running' } }, 202)
    await expect(requestScan()).resolves.toEqual({ id: 's1', state: 'running' })
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: 'POST' })

    fetchMock = mockFetch({ intervalHours: 48, defaultIntervalHours: 24, intervalSource: 'user' })
    await updateScanInterval(48)
    expect(fetchMock.mock.calls[0][1]).toMatchObject({
      method: 'PUT',
      body: JSON.stringify({ intervalHours: 48 }),
    })
  })

  it('reports a scan already running with its French message', async () => {
    mockFetch({ error: { code: 'SCAN_IN_PROGRESS', message: 'running' } }, 409)
    await expect(requestScan()).rejects.toThrow('Une analyse est déjà en cours.')
  })
})
