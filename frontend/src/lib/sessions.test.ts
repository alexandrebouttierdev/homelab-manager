import { describe, expect, it } from 'vitest'
import type { SessionInfo } from './api/security'
import { browserLabel, toSessionRow } from './sessions'

describe('browserLabel', () => {
  it.each([
    ['Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0', 'Firefox · Linux'],
    [
      'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36 Edg/130.0',
      'Edge · Windows',
    ],
    [
      'Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Mobile Safari/537.36',
      'Chrome · Android',
    ],
    [
      'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1',
      'Safari · iOS',
    ],
    ['curl/8.5.0', 'curl'],
    ['', 'Navigateur inconnu'],
  ])('describes %s', (userAgent, label) => {
    expect(browserLabel(userAgent)).toBe(label)
  })
})

describe('toSessionRow', () => {
  const session: SessionInfo = {
    id: 's1',
    userId: 'u1',
    username: 'alice',
    ipAddress: '192.0.2.4',
    userAgent: 'curl/8.5.0',
    authMethod: 'password',
    createdAt: '2026-11-23T08:00:00Z',
    lastSeenAt: '2026-11-23T09:00:00Z',
    expiresAt: '2026-11-23T20:00:00Z',
    current: true,
  }

  it('prepares a session for display', () => {
    const row = toSessionRow(session)
    expect(row.browser).toBe('curl')
    expect(row.authMethod).toBe('Mot de passe')
    expect(row.current).toBe(true)
    expect(row.ipAddress).toBe('192.0.2.4')
  })

  it('shows an unknown address as unavailable', () => {
    expect(toSessionRow({ ...session, ipAddress: '' }).ipAddress).toBe('—')
  })
})
