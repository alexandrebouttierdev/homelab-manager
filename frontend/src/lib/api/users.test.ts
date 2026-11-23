import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  createAccount,
  deleteAccount,
  listAccounts,
  resetAccountPassword,
  updateAccount,
  type Account,
} from './users'

const account: Account = {
  id: '9369be23-5ad3-467d-b954-470b622a1442',
  username: 'alice',
  displayName: 'Alice',
  role: 'viewer',
  disabled: false,
  mustChangePassword: true,
  lastLoginAt: null,
  lastLoginIp: null,
  createdAt: '2026-11-23T10:00:00Z',
}

function mockFetch(body: unknown, status = 200) {
  const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status }))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

describe('accounts API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('lists the accounts', async () => {
    const fetchMock = mockFetch({ users: [account] })
    await expect(listAccounts()).resolves.toEqual([account])
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/users')
  })

  it('creates an account', async () => {
    const fetchMock = mockFetch({ user: account }, 201)
    const input = {
      username: 'alice',
      displayName: 'Alice',
      role: 'viewer' as const,
      password: 'initial-password-1',
    }
    await expect(createAccount(input)).resolves.toEqual(account)
    const [path, init] = fetchMock.mock.calls[0]
    expect(path).toBe('/api/v1/users')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body)).toEqual(input)
  })

  it('sends only the changed fields', async () => {
    const fetchMock = mockFetch({ user: { ...account, disabled: true } })
    await updateAccount(account.id, { disabled: true })
    const [path, init] = fetchMock.mock.calls[0]
    expect(path).toBe(`/api/v1/users/${account.id}`)
    expect(init.method).toBe('PATCH')
    expect(JSON.parse(init.body)).toEqual({ disabled: true })
  })

  it('deletes an account', async () => {
    const fetchMock = mockFetch({ deleted: true })
    await expect(deleteAccount(account.id)).resolves.toEqual({ deleted: true })
    expect(fetchMock.mock.calls[0][1].method).toBe('DELETE')
  })

  it('translates the last-administrator refusal', async () => {
    mockFetch(
      { error: { code: 'LAST_ADMIN', message: 'The last active administrator must remain.' } },
      409,
    )
    await expect(updateAccount(account.id, { role: 'viewer' })).rejects.toThrow(
      'Le dernier administrateur actif ne peut être ni supprimé, ni rétrogradé, ni désactivé.',
    )
  })
})

describe('password reset API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('posts the new initial password', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify({ user: account }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await expect(resetAccountPassword(account.id, 'reset-password-1')).resolves.toEqual(account)
    const [path, init] = fetchMock.mock.calls[0]
    expect(path).toBe(`/api/v1/users/${account.id}/password`)
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body)).toEqual({ password: 'reset-password-1' })
  })
})
