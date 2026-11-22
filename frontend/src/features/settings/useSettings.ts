import { useQuery } from '@tanstack/react-query'
import { getSettings } from '../../lib/api/monitoring'

export const settingsQueryKey = ['settings'] as const

/** Effective collection settings and collector states. */
export function useSettings() {
  const settings = useQuery({ queryKey: settingsQueryKey, queryFn: getSettings })
  return {
    settings: settings.data ?? null,
    isLoading: settings.isLoading,
    error: settings.error,
    retry: () => void settings.refetch(),
  }
}
