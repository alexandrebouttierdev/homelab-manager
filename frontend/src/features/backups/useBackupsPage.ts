import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import type { SegmentedOption } from '../../components/ui/SegmentedTabs'
import { loadState } from '../../hooks/loadState'
import { useSession } from '../../hooks/useSession'
import {
  getBackupConfig,
  getBackupSummary,
  listBackupRuns,
  listBackupSnapshots,
  requestBackup,
  type BackupSummary,
} from '../../lib/api/backups'
import {
  backupHealth,
  describeSchedule,
  describeDetails,
  describeStats,
  formatDuration,
  repositoryLabel,
  runDurationSeconds,
  runFailureText,
  runLabel,
  runStateInfo,
} from '../../lib/backups'
import { useRefreshInterval } from '../../lib/refresh'
import { canManageBackups, canRunBackups } from '../../lib/roles'
import { useBackupConfigForm } from './useBackupConfigForm'
import { useBackupRestore } from './useBackupRestore'

export const backupsQueryKey = ['backups'] as const
/** While an operation runs, the page follows it every 3 s. */
const runningPoll = 3000

export type BackupsTab = 'runs' | 'snapshots' | 'config'

/** Logic and view model of BackupsPage (SPEC_ADMINISTRATION.md § 7). */
export function useBackupsPage() {
  const { user } = useSession()
  const queryClient = useQueryClient()
  const refreshInterval = useRefreshInterval()
  const [tab, setTab] = useState<BackupsTab>('runs')
  // Whether the previous summary showed a run: its end refreshes the lists.
  const wasRunning = useRef(false)

  const summaryQuery = useQuery({
    queryKey: [...backupsQueryKey, 'summary'],
    queryFn: async (): Promise<BackupSummary> => {
      const summary = await getBackupSummary()
      if (wasRunning.current && !summary.running) {
        void queryClient.invalidateQueries({ queryKey: [...backupsQueryKey, 'runs'] })
        void queryClient.invalidateQueries({ queryKey: [...backupsQueryKey, 'snapshots'] })
        void queryClient.invalidateQueries({ queryKey: [...backupsQueryKey, 'config'] })
      }
      wasRunning.current = summary.running !== null
      return summary
    },
    refetchInterval: (query) =>
      query.state.data?.running ? runningPoll : refreshInterval || false,
  })
  const running = summaryQuery.data?.running ?? null
  const runsQuery = useQuery({
    queryKey: [...backupsQueryKey, 'runs'],
    queryFn: () => listBackupRuns(50),
    refetchInterval: running ? runningPoll : refreshInterval || false,
  })
  const snapshotsQuery = useQuery({
    queryKey: [...backupsQueryKey, 'snapshots'],
    queryFn: listBackupSnapshots,
    enabled: tab === 'snapshots',
  })
  const configQuery = useQuery({
    queryKey: [...backupsQueryKey, 'config'],
    queryFn: getBackupConfig,
  })
  const backup = useMutation({
    mutationFn: requestBackup,
    onSettled: () => queryClient.invalidateQueries({ queryKey: backupsQueryKey }),
  })
  const canManage = canManageBackups(user?.role)
  const configForm = useBackupConfigForm(configQuery.data, canManage)
  const restore = useBackupRestore(canManage)

  const summary = summaryQuery.data
  const config = configQuery.data
  const tabs: ReadonlyArray<SegmentedOption<BackupsTab>> = [
    { value: 'runs', label: 'Historique' },
    { value: 'snapshots', label: 'Instantanés' },
    { value: 'config', label: 'Configuration' },
  ]
  const ready = Boolean(summary?.initialized && config && config.sources.length > 0)

  return {
    summary: {
      state: loadState(summaryQuery),
      health: summary ? backupHealth(summary) : null,
      configured: summary?.configured ?? false,
      lastSuccessAt: summary?.lastSuccess?.finishedAt ?? null,
      nextRunAt: summary?.nextRunAt ?? null,
      runningSince: running?.startedAt ?? null,
      runningLabel: running ? runLabel(running) : null,
    },
    secretsMissing: config ? !config.secretsAvailable : false,
    repository: config?.repository
      ? `${repositoryLabel(config.repository.type)} · ${config.repository.location}`
      : null,
    schedule: config ? describeSchedule(config.schedule) : null,
    sourcesCount: config?.sources.length ?? 0,
    backupNow: {
      visible: canRunBackups(user?.role),
      disabled: !ready || running !== null || backup.isPending,
      hint: !summary?.initialized
        ? 'Le dépôt doit être configuré et initialisé.'
        : config && config.sources.length === 0
          ? 'Ajoutez au moins une source.'
          : null,
      loading: backup.isPending,
      run: () => backup.mutate(),
      error: backup.error?.message ?? null,
    },
    tab,
    setTab,
    tabs,
    runs: {
      state: loadState(runsQuery),
      rows: (runsQuery.data ?? []).map((run) => ({
        id: run.id,
        label: runLabel(run),
        state: runStateInfo(run),
        startedAt: run.startedAt,
        duration: run.state === 'running' ? 'en cours' : formatDuration(runDurationSeconds(run)),
        result:
          run.state !== 'succeeded'
            ? null
            : run.operation === 'backup'
              ? describeStats(run.stats)
              : describeDetails(run),
        problem: runFailureText(run),
        requestedBy: run.requestedBy,
        snapshot: run.snapshotId ? run.snapshotId.slice(0, 8) : null,
      })),
    },
    snapshots: { state: loadState(snapshotsQuery), rows: snapshotsQuery.data ?? [] },
    restore,
    configForm,
    refresh: () => void queryClient.invalidateQueries({ queryKey: backupsQueryKey }),
  }
}

export type BackupsPageModel = ReturnType<typeof useBackupsPage>
