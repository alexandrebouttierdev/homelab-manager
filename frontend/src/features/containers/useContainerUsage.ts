import { useQuery } from '@tanstack/react-query'
import { getCurrentContainerMetrics } from '../../lib/api/monitoring'
import { latestOne } from '../../lib/metrics'
import { useRefreshInterval } from '../../lib/refresh'

export type ContainerUsage =
  | { state: 'disabled' | 'loading' | 'error' }
  | {
      state: 'ok'
      cpu: number | null
      memory: number | null
      memoryLimit: number | null
      memoryPercent: number | null
    }

/** Latest CPU and memory of one container (only when enabled). */
export function useContainerUsage(containerId: string, enabled: boolean): ContainerUsage {
  const refreshInterval = useRefreshInterval()
  const metrics = useQuery({
    queryKey: ['container', containerId, 'current-metrics'],
    queryFn: () => getCurrentContainerMetrics(containerId),
    enabled,
    staleTime: 15_000,
    refetchInterval: enabled && refreshInterval > 0 ? refreshInterval : false,
  })
  if (!enabled) return { state: 'disabled' }
  if (metrics.isLoading) return { state: 'loading' }
  if (metrics.isError) return { state: 'error' }
  const samples = metrics.data?.samples ?? []
  return {
    state: 'ok',
    cpu: latestOne(samples, 'cpu.percent')?.value ?? null,
    memory: latestOne(samples, 'memory.usage_bytes')?.value ?? null,
    memoryLimit: latestOne(samples, 'memory.limit_bytes')?.value ?? null,
    memoryPercent: latestOne(samples, 'memory.percent')?.value ?? null,
  }
}
