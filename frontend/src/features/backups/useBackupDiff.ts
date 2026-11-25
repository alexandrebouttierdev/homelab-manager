import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import {
  getBackupRun,
  requestBackupOperation,
  type BackupDiffModifier,
  type BackupSnapshot,
} from '../../lib/api/backups'
import { describeDiff, diffModifierInfo, orderForDiff, runFailureText } from '../../lib/backups'

const backupsQueryKey = ['backups'] as const
/** The comparison is followed every 2 s until it ends. */
const diffPoll = 2000

export type DiffFilter = 'all' | BackupDiffModifier

const filterOptions: ReadonlyArray<{ value: DiffFilter; label: string }> = [
  { value: 'all', label: 'Tout' },
  { value: '+', label: 'Ajoutés' },
  { value: '-', label: 'Supprimés' },
  { value: 'M', label: 'Modifiés' },
  { value: 'U', label: 'Métadonnées' },
]

/**
 * Comparison of two snapshots (administrators, SPEC_ADMINISTRATION.md
 * § 7.4): restic diff runs in the unit, the page follows the run and shows
 * its figures and the first changed paths.
 */
export function useBackupDiff(canManage: boolean) {
  const queryClient = useQueryClient()
  const [selected, setSelected] = useState<BackupSnapshot[]>([])
  const [filter, setFilter] = useState<DiffFilter>('all')
  const [query, setQuery] = useState('')
  const compare = useMutation({
    mutationFn: ([from, to]: [BackupSnapshot, BackupSnapshot]) =>
      requestBackupOperation('diff', { diff: { from: from.id, to: to.id } }),
    onSettled: () => queryClient.invalidateQueries({ queryKey: backupsQueryKey }),
  })
  const runId = compare.data?.id ?? null
  const runQuery = useQuery({
    queryKey: [...backupsQueryKey, 'run', runId],
    queryFn: () => getBackupRun(runId ?? ''),
    enabled: runId !== null,
    refetchInterval: (state) =>
      state.state.data?.state === 'succeeded' || state.state.data?.state === 'failed'
        ? false
        : diffPoll,
  })
  const run = runQuery.data ?? compare.data ?? null
  const diff = run?.details?.diff ?? null
  const needle = query.trim().toLowerCase()
  const changes = (diff?.changes ?? [])
    .filter((change) => filter === 'all' || change.modifier === filter)
    .filter((change) => needle === '' || change.path.toLowerCase().includes(needle))

  return {
    visible: canManage,
    isSelected: (snapshot: BackupSnapshot) => selected.some((s) => s.id === snapshot.id),
    /** Two snapshots at most: a third one replaces the oldest pick. */
    toggle: (snapshot: BackupSnapshot) =>
      setSelected((current) =>
        current.some((s) => s.id === snapshot.id)
          ? current.filter((s) => s.id !== snapshot.id)
          : [...current, snapshot].slice(-2),
      ),
    selectedCount: selected.length,
    canCompare: selected.length === 2 && !compare.isPending,
    compare: () => {
      if (selected.length !== 2) return
      setFilter('all')
      setQuery('')
      compare.mutate(orderForDiff(selected[0], selected[1]))
    },
    pending: compare.isPending,
    error: compare.error?.message ?? runQuery.error?.message ?? null,
    open: run !== null,
    close: () => {
      compare.reset()
      setSelected([])
    },
    result: run && {
      running: run.state === 'running',
      failure: run.state === 'failed' ? runFailureText(run) : null,
      from: diff?.from.slice(0, 8) ?? '',
      to: diff?.to.slice(0, 8) ?? '',
      summary: run.state === 'succeeded' && diff ? describeDiff(diff) : null,
      total: diff?.totalChanges ?? 0,
      truncated: diff?.truncated ?? false,
      /** Paths are served to administrators only: without them, only the figures show. */
      pathsHidden:
        run.state === 'succeeded' &&
        (diff?.totalChanges ?? 0) > 0 &&
        (diff?.changes.length ?? 0) === 0,
      rows: changes.map((change) => ({ ...change, info: diffModifierInfo(change.modifier) })),
    },
    filter,
    setFilter,
    filterOptions,
    query,
    setQuery,
  }
}

export type BackupDiffModel = ReturnType<typeof useBackupDiff>
