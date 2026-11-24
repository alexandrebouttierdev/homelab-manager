import { afterEach, describe, expect, it, vi } from 'vitest'
import { initBackupRepository, listBackupRuns, requestBackup, updateBackupConfig } from './backups'

function mockFetch(body: unknown, status = 200) {
  const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status }))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

describe('backups API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('sends the configuration', async () => {
    const fetchMock = mockFetch({})
    const input = {
      repository: { type: 'local' as const, location: '/mnt/backup' },
      username: null,
      secret: null,
      sources: ['/etc'],
      excludes: [],
      excludeCaches: true,
      excludeIfPresent: '',
      schedule: { enabled: false, frequency: 'daily' as const, weekday: 0, time: '03:00' },
    }
    await updateBackupConfig(input)
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/backups/config')
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: 'PUT', body: JSON.stringify(input) })
  })

  it('initialises the repository and runs a backup', async () => {
    mockFetch({ password: 'secret', run: { id: 'r1' } }, 202)
    await expect(initBackupRepository()).resolves.toMatchObject({ password: 'secret' })
    const fetchMock = mockFetch({ run: { id: 'r2' } }, 202)
    await expect(requestBackup()).resolves.toMatchObject({ id: 'r2' })
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: 'POST' })
  })

  it('lists the runs and explains a run in progress', async () => {
    const fetchMock = mockFetch({ runs: [] })
    await listBackupRuns(20)
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/backups/runs?limit=20')
    mockFetch({ error: { code: 'BACKUP_IN_PROGRESS', message: 'busy' } }, 409)
    await expect(requestBackup()).rejects.toThrow(/déjà en cours/)
  })
})
