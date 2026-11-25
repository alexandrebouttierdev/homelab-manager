import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  auditExportUrl,
  getSecurityPolicy,
  listAudit,
  listSessions,
  revokeOtherSessions,
  revokeSession,
  updateSecurityPolicy,
} from './security'

function mockFetch(body: unknown, status = 200) {
  // A new Response per call: a body can be read only once.
  const fetchMock = vi
    .fn()
    .mockImplementation(() => Promise.resolve(new Response(JSON.stringify(body), { status })))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

describe('security API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('lists the own or every session', async () => {
    const fetchMock = mockFetch({ sessions: [] })
    await listSessions(false)
    await listSessions(true)
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/auth/sessions')
    expect(fetchMock.mock.calls[1][0]).toBe('/api/v1/auth/sessions?scope=all')
  })

  it('revokes one session or the others', async () => {
    const fetchMock = mockFetch({ revoked: 2 })
    await revokeSession('9369be23-5ad3-467d-b954-470b622a1442')
    await expect(revokeOtherSessions()).resolves.toBe(2)
    expect(fetchMock.mock.calls[0][0]).toBe(
      '/api/v1/auth/sessions/9369be23-5ad3-467d-b954-470b622a1442',
    )
    expect(fetchMock.mock.calls[0][1].method).toBe('DELETE')
    expect(fetchMock.mock.calls[1][0]).toBe('/api/v1/auth/sessions/revoke-others')
    expect(fetchMock.mock.calls[1][1].method).toBe('POST')
  })

  it('reads and writes the security policy', async () => {
    const policy = {
      idleTimeoutMinutes: 30,
      allowedNetworks: ['10.0.0.0/8'],
      currentAddress: '10.0.0.2',
    }
    const fetchMock = mockFetch(policy)
    await expect(getSecurityPolicy()).resolves.toEqual(policy)
    await updateSecurityPolicy({
      idleTimeoutMinutes: 30,
      allowedNetworks: ['10.0.0.0/8'],
      requireAdminSecondFactor: false,
      allowPasskeyLogin: false,
    })
    expect(fetchMock.mock.calls[1][1].method).toBe('PUT')
    expect(JSON.parse(fetchMock.mock.calls[1][1].body)).toEqual({
      idleTimeoutMinutes: 30,
      allowedNetworks: ['10.0.0.0/8'],
      requireAdminSecondFactor: false,
      allowPasskeyLogin: false,
    })
  })

  it('filters the audit log and builds export links', async () => {
    const fetchMock = mockFetch({ entries: [] })
    const from = new Date('2026-11-22T10:00:00Z')
    await listAudit({ actor: 'alice', outcome: 'denied', from })
    expect(fetchMock.mock.calls[0][0]).toBe(
      '/api/v1/audit?actor=alice&outcome=denied&from=2026-11-22T10%3A00%3A00.000Z',
    )
    expect(auditExportUrl({ outcome: 'denied' }, 'csv')).toBe(
      '/api/v1/audit/export?outcome=denied&format=csv',
    )
  })

  it('explains a self-exclusion', async () => {
    mockFetch({ error: { code: 'POLICY_EXCLUDES_CURRENT_ADDRESS', message: 'x' } }, 409)
    await expect(
      updateSecurityPolicy({
        idleTimeoutMinutes: 60,
        allowedNetworks: ['10.0.0.0/8'],
        requireAdminSecondFactor: false,
        allowPasskeyLogin: false,
      }),
    ).rejects.toThrow(/votre propre adresse/)
  })
})
