import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from './client'
import {
  createChannel,
  createSilence,
  deleteChannel,
  liftSilence,
  listChannels,
  listSilences,
  testChannel,
  updateChannel,
  type ChannelInput,
} from './notifications'

function mockFetch(body: unknown, status = 200) {
  const fetchMock = vi
    .fn()
    .mockImplementation(() => Promise.resolve(new Response(JSON.stringify(body), { status })))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

const input: ChannelInput = {
  name: 'Téléphone',
  type: 'ntfy',
  enabled: true,
  minSeverity: 'warning',
  renotifyMinutes: 30,
  settings: { serverUrl: 'https://ntfy.sh', topic: 'monoops' },
  secrets: { accessToken: null },
}

describe('notifications API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('lists, creates and lifts silences', async () => {
    let fetchMock = mockFetch({ silences: [] })
    await expect(listSilences()).resolves.toEqual([])
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/alert-silences')

    fetchMock = mockFetch({ silence: { id: 's1' } }, 201)
    const silence = { scope: 'all' as const, durationMinutes: 60, reason: 'maintenance' }
    await expect(createSilence(silence)).resolves.toEqual({ id: 's1' })
    expect(fetchMock.mock.calls[0][1]).toMatchObject({
      method: 'POST',
      body: JSON.stringify(silence),
    })

    fetchMock = mockFetch({ silence: { id: 's/1' } })
    await liftSilence('s/1')
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/alert-silences/s%2F1')
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: 'DELETE' })
  })

  it('manages channels and keeps null secrets in the body', async () => {
    mockFetch({ channels: [], secretsAvailable: false })
    await expect(listChannels()).resolves.toEqual({ channels: [], secretsAvailable: false })

    let fetchMock = mockFetch({ channel: { id: 'c1' } }, 201)
    await createChannel(input)
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/notification-channels')
    expect(JSON.parse(fetchMock.mock.calls[0][1].body as string).secrets).toEqual({
      accessToken: null,
    })

    fetchMock = mockFetch({ channel: { id: 'c1' } })
    await updateChannel('c1', input)
    expect(fetchMock.mock.calls[0]).toEqual([
      '/api/v1/notification-channels/c1',
      expect.objectContaining({ method: 'PUT' }),
    ])

    fetchMock = mockFetch({ deleted: true })
    await deleteChannel('c1')
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: 'DELETE' })
  })

  it('reports a failed test with its French message', async () => {
    mockFetch({ error: { code: 'NOTIFICATION_FAILED', message: 'delivery failed: timeout.' } }, 502)
    const failure = testChannel('c1')
    await expect(failure).rejects.toBeInstanceOf(ApiError)
    await expect(failure).rejects.toThrow("L'envoi de test a échoué")
  })
})
