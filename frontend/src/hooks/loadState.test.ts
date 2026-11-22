import type { UseQueryResult } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { loadState } from './loadState'

function query(partial: Partial<UseQueryResult<unknown>>) {
  return {
    isLoading: false,
    isError: false,
    error: null,
    refetch: vi.fn(),
    ...partial,
  } as UseQueryResult<unknown>
}

describe('loadState', () => {
  it('exposes the error only when the query failed', () => {
    const failed = loadState(query({ isError: true, error: new Error('boom') }))
    expect(failed.isError && failed.error.message).toBe('boom')
    const ok = loadState(query({ isLoading: true }))
    expect(ok).toMatchObject({ isLoading: true, isError: false, error: null })
  })

  it('retries through refetch', () => {
    const refetch = vi.fn()
    loadState(query({ refetch })).refetch()
    expect(refetch).toHaveBeenCalledOnce()
  })
})
