import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { requestBackupOperation, type BackupSnapshot } from '../../lib/api/backups'
import { splitLines } from '../../schemas/backupConfig'

const backupsQueryKey = ['backups'] as const
const absolutePath = /^\/(?!.*\/\/)(?!.*\/$)(?!.*(^|\/)\.\.(\/|$)).*$/

/**
 * Restore of a snapshot (administrators): a simulation first, then the
 * restore into the quarantine directory of the run (SPEC_ADMINISTRATION.md
 * § 7.4). The original location is never written.
 */
export function useBackupRestore(canManage: boolean) {
  const queryClient = useQueryClient()
  const [snapshot, setSnapshot] = useState<BackupSnapshot | null>(null)
  const [include, setInclude] = useState('')
  const mutation = useMutation({
    mutationFn: (dryRun: boolean) =>
      requestBackupOperation('restore', {
        restore: { snapshotId: snapshot?.id ?? '', include: splitLines(include), dryRun },
      }),
    onSettled: () => queryClient.invalidateQueries({ queryKey: backupsQueryKey }),
  })
  const paths = splitLines(include)
  const invalid = paths.find((path) => !absolutePath.test(path)) ?? null

  return {
    visible: canManage,
    snapshot,
    choose: (next: BackupSnapshot) => {
      mutation.reset()
      setInclude('')
      setSnapshot(next)
    },
    close: () => setSnapshot(null),
    include,
    setInclude,
    includeError: invalid ? `« ${invalid} » n'est pas un chemin absolu propre` : null,
    simulate: () => mutation.mutate(true),
    restore: () => mutation.mutate(false),
    pending: mutation.isPending ? (mutation.variables ? 'simulate' : 'restore') : null,
    started: mutation.data
      ? mutation.data.details?.dryRun
        ? "Simulation lancée : son résultat apparaît dans l'historique."
        : `Restauration lancée dans ${mutation.data.details?.target ?? 'la quarantaine'} : suivez-la dans l'historique.`
      : null,
    error: mutation.error?.message ?? null,
  }
}

export type BackupRestoreModel = ReturnType<typeof useBackupRestore>
