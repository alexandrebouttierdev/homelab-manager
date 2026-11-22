import { useQuery } from '@tanstack/react-query'
import { getHost } from '../lib/api/monitoring'

/** Host identity and system collector state, shared by the layout. */
export function useHost() {
  const host = useQuery({ queryKey: ['host'], queryFn: getHost, staleTime: 30_000, retry: false })
  const data = host.data ?? null
  return {
    host: data,
    online: data?.collector.status === 'ok',
    /** "Distribution version", or Linux when unknown. */
    osLabel: data ? `${data.os.distribution || 'Linux'} ${data.os.version || ''}`.trim() : null,
  }
}
