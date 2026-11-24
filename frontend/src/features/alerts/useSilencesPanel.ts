import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo } from 'react'
import { loadState } from '../../hooks/loadState'
import { useSession } from '../../hooks/useSession'
import { liftSilence, listSilences } from '../../lib/api/notifications'
import { toSilenceRow } from '../../lib/notifications'
import { useRefreshInterval } from '../../lib/refresh'
import { canOperateAlerts } from '../../lib/roles'
import { alertsQueryKey } from './queryKeys'
import { useSilenceForm } from './useSilenceForm'

/** Logic and view model of the silences tab of AlertsPage. */
export function useSilencesPanel() {
  const refreshInterval = useRefreshInterval()
  const queryClient = useQueryClient()
  const { user } = useSession()
  const silencesQuery = useQuery({
    queryKey: [...alertsQueryKey, 'silences'],
    queryFn: listSilences,
    refetchInterval: refreshInterval || false,
  })
  const lift = useMutation({
    mutationFn: liftSilence,
    onSettled: () => queryClient.invalidateQueries({ queryKey: alertsQueryKey }),
  })
  const silence = useSilenceForm()
  const rows = useMemo(() => {
    const now = new Date()
    return (silencesQuery.data ?? []).map((item) => toSilenceRow(item, now))
  }, [silencesQuery.data])

  return {
    state: loadState(silencesQuery),
    rows,
    canOperate: canOperateAlerts(user?.role),
    silence,
    startGlobal: () => silence.open(null),
    lift: (id: string) => lift.mutate(id),
    liftingId: lift.isPending ? (lift.variables ?? null) : null,
    liftError: lift.error?.message ?? null,
  }
}

export type SilencesPanelModel = ReturnType<typeof useSilencesPanel>
