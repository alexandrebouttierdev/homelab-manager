import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import type { SegmentedOption } from '../../components/ui/SegmentedTabs'
import { loadState } from '../../hooks/loadState'
import { useSession } from '../../hooks/useSession'
import { formatMeanResolution, sortOpenAlerts, toAlertRow } from '../../lib/alerts'
import { acknowledgeAlert, getAlertSummary, listAlerts } from '../../lib/api/alerts'
import { useRefreshInterval } from '../../lib/refresh'
import { canOperateAlerts } from '../../lib/roles'

export type AlertsTab = 'alerts' | 'rules'
export type AlertsView = 'open' | 'history'

export const alertsTabs: ReadonlyArray<SegmentedOption<AlertsTab>> = [
  { value: 'alerts', label: 'Alertes' },
  { value: 'rules', label: 'Règles' },
]

export const alertsQueryKey = ['alerts'] as const

/** Logic and view model of the alerts tab of AlertsPage. */
export function useAlertsPage() {
  const refreshInterval = useRefreshInterval()
  const queryClient = useQueryClient()
  const { user } = useSession()
  const [view, setView] = useState<AlertsView>('open')

  const summaryQuery = useQuery({
    queryKey: [...alertsQueryKey, 'summary'],
    queryFn: getAlertSummary,
    refetchInterval: refreshInterval || false,
  })
  const openQuery = useQuery({
    queryKey: [...alertsQueryKey, 'open'],
    queryFn: () => listAlerts('open'),
    refetchInterval: refreshInterval || false,
  })
  const historyQuery = useQuery({
    queryKey: [...alertsQueryKey, 'resolved'],
    queryFn: () => listAlerts('resolved'),
    enabled: view === 'history',
    refetchInterval: refreshInterval || false,
  })
  const acknowledge = useMutation({
    mutationFn: acknowledgeAlert,
    onSettled: () => queryClient.invalidateQueries({ queryKey: alertsQueryKey }),
  })

  const openRows = useMemo(() => {
    const now = new Date()
    return sortOpenAlerts(openQuery.data ?? []).map((alert) => toAlertRow(alert, now))
  }, [openQuery.data])
  const historyRows = useMemo(() => {
    const now = new Date()
    return (historyQuery.data ?? []).map((alert) => toAlertRow(alert, now))
  }, [historyQuery.data])
  const summary = summaryQuery.data

  return {
    view,
    setView,
    viewOptions: [
      { value: 'open' as const, label: 'Ouvertes', count: openRows.length },
      { value: 'history' as const, label: 'Historique' },
    ],
    summary: summary
      ? {
          critical: summary.open.critical,
          warning: summary.open.warning,
          info: summary.open.info,
          resolvedLast24h: summary.resolvedLast24h,
          meanResolution: formatMeanResolution(summary.meanResolutionSeconds24h),
        }
      : null,
    open: { state: loadState(openQuery), rows: openRows },
    history: { state: loadState(historyQuery), rows: historyRows },
    canAcknowledge: canOperateAlerts(user?.role),
    acknowledge: (id: string) => acknowledge.mutate(id),
    acknowledgingId: acknowledge.isPending ? (acknowledge.variables ?? null) : null,
    acknowledgeError: acknowledge.error?.message ?? null,
    refresh: () => void queryClient.invalidateQueries({ queryKey: alertsQueryKey }),
  }
}

export type AlertsPageModel = ReturnType<typeof useAlertsPage>
