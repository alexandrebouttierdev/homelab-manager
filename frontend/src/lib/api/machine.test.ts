import { afterEach, describe, expect, it, vi } from 'vitest'
import { getPowerSettings, requestPowerAction } from './machine'

function mockFetch(body: unknown, status = 200) {
  const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status }))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

describe('machine power API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('reads whether power actions are enabled', async () => {
    mockFetch({ enabled: false, actions: ['reboot', 'poweroff'] })
    await expect(getPowerSettings()).resolves.toEqual({
      enabled: false,
      actions: ['reboot', 'poweroff'],
    })
  })

  it('sends the typed host name with the action', async () => {
    const fetchMock = mockFetch({ requested: true, action: 'reboot' }, 202)
    await requestPowerAction('reboot', 'homelab')
    const [path, init] = fetchMock.mock.calls[0]
    expect(path).toBe('/api/v1/machine/power/reboot')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body)).toEqual({ confirmHostname: 'homelab' })
  })

  it('explains a polkit refusal', async () => {
    mockFetch({ error: { code: 'POWER_ACTION_REFUSED', message: 'refused' } }, 503)
    await expect(requestPowerAction('poweroff', 'homelab')).rejects.toThrow(/polkit/)
  })
})
