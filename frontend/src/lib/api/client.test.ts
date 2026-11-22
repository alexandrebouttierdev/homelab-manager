import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, errorMessages, isUnauthenticated, requestJSON } from './client'

function mockFetch(status: number, body: unknown) {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status })))
}

describe('requestJSON', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('translates API error codes into French messages', async () => {
    mockFetch(429, { error: { code: 'RATE_LIMITED', message: 'Too many login attempts.' } })
    const error = await requestJSON('/api/v1/auth/login').catch((caught: unknown) => caught)
    expect(error).toBeInstanceOf(ApiError)
    expect((error as ApiError).message).toBe(errorMessages.RATE_LIMITED)
    expect((error as ApiError).code).toBe('RATE_LIMITED')
  })

  it('falls back to a generic French message for unknown codes', async () => {
    mockFetch(500, { error: { code: 'SOMETHING_NEW', message: 'Internal detail' } })
    await expect(requestJSON('/api/v1/dashboard')).rejects.toThrow('Une erreur est survenue.')
  })

  it('reports a network failure without leaking details', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')))
    await expect(requestJSON('/api/v1/dashboard')).rejects.toThrow(
      'Impossible de joindre le Manager local.',
    )
  })
})

describe('isUnauthenticated', () => {
  it('only matches a missing or revoked session', () => {
    expect(isUnauthenticated(new ApiError(401, '', 'SESSION_NOT_FOUND'))).toBe(true)
    expect(isUnauthenticated(new ApiError(401, '', 'INVALID_CREDENTIALS'))).toBe(false)
    expect(isUnauthenticated(new ApiError(503, '', 'AUTH_UNAVAILABLE'))).toBe(false)
    expect(isUnauthenticated(new Error('boom'))).toBe(false)
  })
})
