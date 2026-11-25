import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { getBackupRun, requestBackupOperation, type BackupSnapshot } from '../../lib/api/backups'
import { archiveUrl, describeDetails, inPlaceIssue } from '../../lib/backups'
import { splitLines } from '../../schemas/backupConfig'

const backupsQueryKey = ['backups'] as const
const absolutePath = /^\/(?!.*\/\/)(?!.*\/$)(?!.*(^|\/)\.\.(\/|$)).*$/
const pollInterval = 2000

export type RestoreDestination = 'quarantine' | 'in-place'

/** Follows one run until it ends. */
function useFollowedRun(id: string | null) {
  return useQuery({
    queryKey: ['backups', 'run', id],
    queryFn: () => getBackupRun(id ?? ''),
    enabled: id !== null,
    refetchInterval: (query) => (query.state.data?.state === 'running' ? pollInterval : false),
  })
}

/**
 * Restore of a snapshot (administrators): a simulation first, then the
 * restore into the quarantine directory of the run (SPEC_ADMINISTRATION.md
 * § 7.4), or, after a successful simulation of the same paths and an
 * acknowledgement, to their original location (decision D-2); and the
 * archive of one path to download.
 */
export function useBackupRestore(canManage: boolean) {
  const queryClient = useQueryClient()
  const [snapshot, setSnapshot] = useState<BackupSnapshot | null>(null)
  const [include, setInclude] = useState('')
  const [destination, setDestination] = useState<RestoreDestination>('quarantine')
  const [acknowledged, setAcknowledged] = useState(false)
  // The in-place simulation of this panel, with the paths it covered.
  const [simulation, setSimulation] = useState<{ id: string; paths: string } | null>(null)
  const [archivePath, setArchivePath] = useState('')
  const [archiveRun, setArchiveRun] = useState<string | null>(null)

  const paths = splitLines(include)
  const pathKey = [...paths].sort().join('\n')
  const mutation = useMutation({
    mutationFn: (dryRun: boolean) =>
      requestBackupOperation(destination === 'in-place' ? 'restore-in-place' : 'restore', {
        restore: { snapshotId: snapshot?.id ?? '', include: paths, dryRun },
      }),
    onSuccess: (run, dryRun) => {
      if (destination === 'in-place' && dryRun) setSimulation({ id: run.id, paths: pathKey })
      if (destination === 'in-place' && !dryRun) {
        setSimulation(null)
        setAcknowledged(false)
      }
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: backupsQueryKey }),
  })
  const dump = useMutation({
    mutationFn: () =>
      requestBackupOperation('dump', {
        dump: { snapshotId: snapshot?.id ?? '', path: archivePath.trim() },
      }),
    onSuccess: (run) => setArchiveRun(run.id),
    onSettled: () => queryClient.invalidateQueries({ queryKey: backupsQueryKey }),
  })
  const simulationRun = useFollowedRun(simulation?.id ?? null)
  const archive = useFollowedRun(archiveRun)

  const invalid = paths.find((path) => !absolutePath.test(path)) ?? null
  const inPlace = destination === 'in-place'
  const inPlaceError = inPlace && !invalid ? inPlaceIssue(paths) : null
  const simulated =
    simulation !== null && simulation.paths === pathKey && simulationRun.data?.state === 'succeeded'
  const archiveError =
    archivePath.trim() !== '' && !absolutePath.test(archivePath.trim())
      ? `« ${archivePath.trim()} » n'est pas un chemin absolu propre`
      : null
  const archiveData = archive.data

  return {
    visible: canManage,
    snapshot,
    choose: (next: BackupSnapshot) => {
      mutation.reset()
      dump.reset()
      setInclude('')
      setDestination('quarantine')
      setAcknowledged(false)
      setSimulation(null)
      setArchivePath('')
      setArchiveRun(null)
      setSnapshot(next)
    },
    close: () => setSnapshot(null),
    include,
    setInclude,
    destination,
    setDestination: (next: RestoreDestination) => {
      mutation.reset()
      setAcknowledged(false)
      setDestination(next)
    },
    includeError: invalid
      ? `« ${invalid} » n'est pas un chemin absolu propre`
      : (inPlaceError ?? null),
    simulate: () => mutation.mutate(true),
    restore: () => mutation.mutate(false),
    /** In place: the simulation of these paths, then the acknowledgement. */
    simulation: inPlace
      ? simulationRun.data
        ? {
            running: simulationRun.data.state === 'running',
            result:
              simulationRun.data.state === 'succeeded'
                ? describeDetails(simulationRun.data)
                : simulationRun.data.state === 'failed'
                  ? (simulationRun.data.message ?? 'La simulation a échoué.')
                  : null,
            current: simulation?.paths === pathKey,
          }
        : null
      : null,
    acknowledged,
    setAcknowledged,
    canRestore: inPlace ? simulated && acknowledged : true,
    pending: mutation.isPending ? (mutation.variables ? 'simulate' : 'restore') : null,
    started: mutation.data
      ? mutation.data.details?.dryRun
        ? "Simulation lancée : son résultat apparaît ci-dessous et dans l'historique."
        : inPlace || mutation.data.operation === 'restore-in-place'
          ? "Restauration à l'emplacement d'origine lancée, après une sauvegarde de sécurité des fichiers remplacés : suivez-la dans l'historique."
          : `Restauration lancée dans ${mutation.data.details?.target ?? 'la quarantaine'} : suivez-la dans l'historique.`
      : null,
    error: mutation.error?.message ?? null,
    archivePath,
    setArchivePath,
    archiveError,
    prepareArchive: () => dump.mutate(),
    archivePending: dump.isPending || archiveData?.state === 'running',
    archiveFailure:
      dump.error?.message ??
      (archiveData?.state === 'failed'
        ? (archiveData.message ?? "L'archive n'a pas pu être préparée.")
        : null),
    archive: archiveData
      ? { url: archiveUrl(archiveData, new Date()), description: describeDetails(archiveData) }
      : null,
  }
}

export type BackupRestoreModel = ReturnType<typeof useBackupRestore>
