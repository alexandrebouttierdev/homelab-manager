import { afterEach, describe, expect, it, vi } from 'vitest'
import { completeSecondFactor, login, needsSecondFactor } from './auth'
import {
  beginTotp,
  confirmTotp,
  disableSecondFactor,
  getSecondFactorStatus,
  regenerateRecoveryCodes,
} from './secondFactor'

function mockFetch(body: unknown, status = 200) {
  const fetchMock = vi
    .fn()
    .mockImplementation(() => Promise.resolve(new Response(JSON.stringify(body), { status })))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

describe('second factor API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('tells a challenge from a session at login', async () => {
    mockFetch({ secondFactorRequired: true, methods: ['totp', 'recovery'], expiresAt: 'x' })
    const challenge = await login('alice', 'password')
    expect(needsSecondFactor(challenge)).toBe(true)
    mockFetch({ user: { id: 'a', username: 'alice' } })
    expect(needsSecondFactor(await login('alice', 'password'))).toBe(false)
  })

  it('completes the login with a code', async () => {
    const fetchMock = mockFetch({ user: { id: 'a' } })
    await completeSecondFactor('recovery', 'k7pq-2mzx-b9ht')
    const [path, init] = fetchMock.mock.calls[0]
    expect(path).toBe('/api/v1/auth/login/second-factor')
    expect(JSON.parse(init.body)).toEqual({ method: 'recovery', code: 'k7pq-2mzx-b9ht' })
  })

  it('manages the enrollment', async () => {
    const fetchMock = mockFetch({
      recoveryCodes: ['a'],
      secret: 'S',
      uri: 'u',
      qr: { size: 21, rows: [] },
    })
    await getSecondFactorStatus()
    await beginTotp('password-1')
    await expect(confirmTotp('123456')).resolves.toEqual(['a'])
    await expect(regenerateRecoveryCodes('password-1')).resolves.toEqual(['a'])
    await disableSecondFactor('password-1')
    expect(fetchMock.mock.calls.map(([path]) => path)).toEqual([
      '/api/v1/auth/second-factor',
      '/api/v1/auth/second-factor/totp',
      '/api/v1/auth/second-factor/totp/confirm',
      '/api/v1/auth/second-factor/recovery-codes',
      '/api/v1/auth/second-factor/disable',
    ])
    expect(JSON.parse(fetchMock.mock.calls[2][1].body)).toEqual({ code: '123456' })
    expect(JSON.parse(fetchMock.mock.calls[4][1].body)).toEqual({ password: 'password-1' })
  })

  it('explains an expired challenge', async () => {
    mockFetch({ error: { code: 'LOGIN_CHALLENGE_EXPIRED', message: 'x' } }, 401)
    await expect(completeSecondFactor('totp', '123456')).rejects.toThrow(/reconnectez-vous/)
  })
})
