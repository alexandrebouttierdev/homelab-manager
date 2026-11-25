import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  connectBackupCopy,
  connectBackupRepository,
  initBackupCopy,
  initBackupRepository,
  listBackupRuns,
  requestBackup,
  removeBackupCopy,
  requestBackupOperation,
  setBackupCopy,
  updateBackupConfig,
} from './backups'

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
      sqliteDatabases: [],
      schedule: { enabled: false, frequency: 'daily' as const, weekday: 0, time: '03:00' },
      retention: { enabled: false, keepDaily: 7, keepWeekly: 4, keepMonthly: 6, keepYearly: 0 },
      check: { enabled: true, subsetPercent: 5 },
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

  it('starts an operation and connects a repository', async () => {
    let fetchMock = mockFetch({ run: { id: 'r3' } }, 202)
    const restore = { snapshotId: '3a253ad8', include: ['/etc'], dryRun: true }
    await requestBackupOperation('restore', { restore })
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/backups/operations')
    expect(fetchMock.mock.calls[0][1]).toMatchObject({
      body: JSON.stringify({ operation: 'restore', restore }),
    })
    fetchMock = mockFetch({ run: { id: 'r4' } }, 202)
    await requestBackupOperation('unlock')
    expect(fetchMock.mock.calls[0][1]).toMatchObject({
      body: JSON.stringify({ operation: 'unlock' }),
    })
    fetchMock = mockFetch({ run: { id: 'r5' } }, 202)
    await connectBackupRepository('old-password')
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/backups/repository/connect')
    fetchMock = mockFetch({ run: { id: 'r6' } }, 202)
    await requestBackupOperation('diff', { diff: { from: '3a253ad8', to: '9f2c11b0' } })
    expect(fetchMock.mock.calls[0][1]).toMatchObject({
      body: JSON.stringify({ operation: 'diff', diff: { from: '3a253ad8', to: '9f2c11b0' } }),
    })
  })

  it('sets, creates, connects and removes the copy repository', async () => {
    let fetchMock = mockFetch({ copy: null })
    const input = {
      repository: { type: 'rest' as const, location: 'https://nas.lan:8000/copy' },
      username: 'u',
      secret: null,
      afterBackup: true,
    }
    await setBackupCopy(input)
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/backups/copy')
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: 'PUT', body: JSON.stringify(input) })
    fetchMock = mockFetch({ password: 'p', run: { id: 'r7' } }, 202)
    expect((await initBackupCopy()).password).toBe('p')
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/backups/copy/init')
    fetchMock = mockFetch({ run: { id: 'r8' } }, 202)
    await connectBackupCopy('old')
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ body: JSON.stringify({ password: 'old' }) })
    fetchMock = mockFetch({ copy: null })
    await removeBackupCopy()
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/backups/copy/remove')
    mockFetch({ error: { code: 'BACKUP_COPY_NOT_INITIALIZED', message: 'x' } }, 409)
    await expect(requestBackupOperation('copy')).rejects.toThrow(/ni créé ni rattaché/)
  })
})
