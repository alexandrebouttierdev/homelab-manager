import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import type { SegmentedOption } from '../../components/ui/SegmentedTabs'
import { loadState } from '../../hooks/loadState'
import { useSession } from '../../hooks/useSession'
import { formatMeanResolution, sortOpenAlerts, toAlertRow } from '../../lib/alerts'
import { acknowledgeAlert, getAlertSummary, listAlerts } from '../../lib/api/alerts'
import { listUnits } from '../../lib/api/services'
import { useRefreshInterval } from '../../lib/refresh'
import { canManageAlerts, canManageServices, canOperateAlerts } from '../../lib/roles'
import { silenceSubject } from '../../lib/notifications'
import { useUnitAction } from '../services/useUnitAction'
import { alertsQueryKey } from './queryKeys'
import { useSilenceForm } from './useSilenceForm'

export { alertsQueryKey }

export type AlertsTab = 'alerts' | 'rules' | 'silences' | 'channels'
export type AlertsView = 'open' | 'history'

/** Sections of AlertsPage; the channels are for administrators only. */
export function useAlertsTabs() {
  const { user } = useSession()
  const [tab, setTab] = useState<AlertsTab>('alerts')
  const options: ReadonlyArray<SegmentedOption<AlertsTab>> = [
    { value: 'alerts', label: 'Alertes' },
    { value: 'rules', label: 'Règles' },
    { value: 'silences', label: 'Silences' },
    ...(canManageAlerts(user?.role) ? [{ value: 'channels' as const, label: 'Canaux' }] : []),
  ]
  return { tab, setTab, options }
}

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
  // The allow list of the service actions, read for the users who may act.
  const canRestart = canManageServices(user?.role)
  const unitsQuery = useQuery({
    queryKey: ['system', 'units'],
    queryFn: listUnits,
    enabled: canRestart,
    staleTime: 30_000,
  })
  const manageable = unitsQuery.data?.manageableUnits
  const unitAction = useUnitAction()
  const silence = useSilenceForm()
  const acknowledge = useMutation({
    mutationFn: acknowledgeAlert,
    onSettled: () => queryClient.invalidateQueries({ queryKey: alertsQueryKey }),
  })

  const openRows = useMemo(() => {
    const now = new Date()
    const restartable = canRestart ? (manageable ?? []) : []
    return sortOpenAlerts(openQuery.data ?? []).map((alert) => toAlertRow(alert, now, restartable))
  }, [openQuery.data, canRestart, manageable])
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
    canSilence: canOperateAlerts(user?.role),
    silence,
    startSilence: (id: string) => {
      const alert = openQuery.data?.find((candidate) => candidate.id === id)
      if (alert) silence.open(silenceSubject(alert))
    },
    unitAction,
    restart: (unit: string) => unitAction.choose({ unit, verb: 'restart', label: 'Redémarrer' }),
    refresh: () => void queryClient.invalidateQueries({ queryKey: alertsQueryKey }),
  }
}

export type AlertsPageModel = ReturnType<typeof useAlertsPage>
