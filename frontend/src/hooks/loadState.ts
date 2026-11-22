import type { UseQueryResult } from '@tanstack/react-query'

/** Loading state of a query, without its data or TanStack internals. */
export type LoadState = { isLoading: boolean; refetch: () => void } & (
  { isError: true; error: Error } | { isError: false; error: null }
)

export function loadState(query: UseQueryResult<unknown>): LoadState {
  const common = { isLoading: query.isLoading, refetch: () => void query.refetch() }
  return query.isError
    ? { ...common, isError: true, error: query.error }
    : { ...common, isError: false, error: null }
}
