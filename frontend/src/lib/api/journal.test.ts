import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  getJournalEntry,
  getJournalSummary,
  journalExportUrl,
  listJournal,
  listJournalBoots,
  listJournalUnits,
  type JournalFilter,
} from './journal'

const cursor = 's=0;i=1e;b=bb;m=1;t=1;x=1'

function mockFetch(body: unknown) {
  const fetchMock = vi
    .fn()
    .mockImplementation(() => Promise.resolve(new Response(JSON.stringify(body), { status: 200 })))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

const defaults: JournalFilter = { source: 'all', priority: 'info' }

describe('journal API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('leaves the server defaults out of the query', async () => {
    const fetchMock = mockFetch({ entries: [] })
    await listJournal(defaults)
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/journal/entries')
  })

  it('sends the filters and escapes the cursors', async () => {
    const fetchMock = mockFetch({ entries: [] })
    await listJournal(
      {
        source: 'kernel',
        priority: 'err',
        unit: 'ssh.service',
        q: 'a b',
        from: '2026-11-23T09:00:00.000Z',
      },
      { before: cursor, limit: 50 },
    )
    const url = new URL(String(fetchMock.mock.calls[0][0]), 'http://manager')
    expect(url.pathname).toBe('/api/v1/journal/entries')
    expect(Object.fromEntries(url.searchParams)).toEqual({
      source: 'kernel',
      priority: 'err',
      unit: 'ssh.service',
      q: 'a b',
      from: '2026-11-23T09:00:00.000Z',
      before: cursor,
      limit: '50',
    })
    // The server refuses an unescaped ";".
    expect(String(fetchMock.mock.calls[0][0])).not.toContain(';')
  })

  it('reads one entry, the boots, the units and the summary', async () => {
    const fetchMock = mockFetch({ boots: [], units: ['ssh.service'] })
    await getJournalEntry(cursor)
    await expect(listJournalBoots()).resolves.toEqual([])
    await expect(listJournalUnits()).resolves.toEqual(['ssh.service'])
    await getJournalSummary()
    expect(fetchMock.mock.calls.map((call) => call[0])).toEqual([
      `/api/v1/journal/entry?cursor=${encodeURIComponent(cursor)}`,
      '/api/v1/journal/boots',
      '/api/v1/journal/units',
      '/api/v1/journal/summary',
    ])
  })

  it('builds the export links', () => {
    expect(journalExportUrl(defaults, 'log')).toBe('/api/v1/journal/export?format=log')
    expect(journalExportUrl({ source: 'docker', priority: 'debug' }, 'json')).toBe(
      '/api/v1/journal/export?source=docker&priority=debug&format=json',
    )
  })
})
