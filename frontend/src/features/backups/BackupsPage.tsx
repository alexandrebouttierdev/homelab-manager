import {
  AlertTriangle,
  DatabaseBackup,
  GitCompare,
  History,
  Play,
  RefreshCw,
  RotateCcw,
  X,
} from 'lucide-react'
import type { ReactNode } from 'react'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/DataTable'
import { EmptyState } from '../../components/ui/EmptyState'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { PageHeader } from '../../components/ui/PageHeader'
import { SegmentedTabs } from '../../components/ui/SegmentedTabs'
import { formatBytes, formatCount, formatDateTime, formatRelativeTime } from '../../lib/format'
import { FormField, inputClass } from '../alerts/FormField'
import { BackupConfigForm } from './BackupConfigForm'
import { BackupCopyCard } from './BackupCopyCard'
import { BackupSSHKeyCard } from './BackupSSHKeyCard'
import { BackupDiffPanel } from './BackupDiffPanel'
import type { BackupRestoreModel } from './useBackupRestore'
import { useBackupsPage, type BackupsPageModel } from './useBackupsPage'

export function BackupsPage() {
  const page = useBackupsPage()
  const { summary } = page
  return (
    <div className="space-y-4">
      <PageHeader
        title="Sauvegardes"
        breadcrumb={[{ label: 'Système' }, { label: 'Sauvegardes' }]}
        badge={summary.health && <Badge tone={summary.health.tone}>{summary.health.label}</Badge>}
        description="Sauvegardes chiffrées par restic, exécutées par l'unité dédiée monoops-backup@.service."
        actions={
          <div className="flex gap-2">
            <Button
              size="sm"
              variant="secondary"
              icon={<RefreshCw className="h-3.5 w-3.5" />}
              onClick={page.refresh}
            >
              Actualiser
            </Button>
            {page.backupNow.visible && (
              <Button
                size="sm"
                variant="primary"
                icon={<Play className="h-3.5 w-3.5" />}
                disabled={page.backupNow.disabled}
                loading={page.backupNow.loading}
                title={page.backupNow.hint ?? undefined}
                onClick={page.backupNow.run}
              >
                Sauvegarder maintenant
              </Button>
            )}
          </div>
        }
      />

      {summary.state.isLoading && <LoadingState label="Lecture des sauvegardes…" rows={3} />}
      {summary.state.isError && (
        <ErrorState message={summary.state.error.message} onRetry={summary.state.refetch} />
      )}
      {page.backupNow.error && (
        <p className="text-xs text-danger" role="alert">
          {page.backupNow.error}
        </p>
      )}
      {page.secretsMissing && (
        <Notice>
          La clé du coffre de secrets est absente : le mot de passe du dépôt ne peut être ni
          conservé ni relu, et aucune sauvegarde ne peut s&apos;exécuter (voir le README).
        </Notice>
      )}
      {summary.runningLabel && (
        <p className="text-xs text-info" role="status">
          {summary.runningLabel} en cours depuis {formatRelativeTime(summary.runningSince)}…
        </p>
      )}

      {summary.health && (
        <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
          <Fact label="État" value={summary.health.label} detail={summary.health.detail} />
          <Fact
            label="Dernier succès"
            value={summary.lastSuccessAt ? formatRelativeTime(summary.lastSuccessAt) : '—'}
            detail={summary.lastSuccessAt ? formatDateTime(summary.lastSuccessAt) : 'Aucun'}
          />
          <Fact
            label="Prochaine sauvegarde"
            value={summary.nextRunAt ? formatDateTime(summary.nextRunAt) : '—'}
            detail={page.schedule ?? ''}
          />
          <Fact
            label="Dépôt"
            value={
              page.repository ? `${formatCount(page.sourcesCount)} source(s)` : 'Non configuré'
            }
            detail={page.repository ?? ''}
          />
        </div>
      )}

      <SegmentedTabs
        options={page.tabs}
        value={page.tab}
        onChange={page.setTab}
        ariaLabel="Sections"
      />
      {page.tab === 'runs' && <Runs page={page} />}
      {page.tab === 'snapshots' && <Snapshots page={page} />}
      {page.tab === 'config' && (
        <div className="space-y-4">
          <BackupSSHKeyCard model={page.sshKey} />
          <BackupConfigForm model={page.configForm} />
          <BackupCopyCard model={page.copy} />
        </div>
      )}
    </div>
  )
}

function Notice({ children }: { children: ReactNode }) {
  return (
    <div
      className="flex items-start gap-2 rounded-md border border-warning/40 bg-warning/5 px-3 py-2.5 text-xs text-text-secondary"
      role="status"
    >
      <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-warning" aria-hidden="true" />
      <p>{children}</p>
    </div>
  )
}

function Fact({ label, value, detail }: { label: string; value: string; detail: string }) {
  return (
    <div className="min-w-0 rounded-lg border border-line bg-card p-4">
      <p className="text-xs text-text-muted">{label}</p>
      <p className="mt-1 truncate text-sm font-semibold text-text-primary">{value}</p>
      <p className="mt-0.5 truncate text-xs text-text-muted" title={detail}>
        {detail}
      </p>
    </div>
  )
}

function Runs({ page }: { page: BackupsPageModel }) {
  const { state, rows } = page.runs
  if (state.isLoading) return <LoadingState label="Chargement de l'historique…" rows={3} />
  if (state.isError) return <ErrorState message={state.error.message} onRetry={state.refetch} />
  if (rows.length === 0)
    return (
      <EmptyState
        icon={History}
        title="Aucune exécution"
        description="Configurez un dépôt et des sources dans l'onglet Configuration, puis initialisez le dépôt."
      />
    )
  return (
    <Table minWidthClassName="min-w-[760px]">
      <THead>
        <tr>
          <TH>Opération</TH>
          <TH>État</TH>
          <TH>Début</TH>
          <TH>Durée</TH>
          <TH>Résultat</TH>
        </tr>
      </THead>
      <TBody>
        {rows.map((row) => (
          <TR key={row.id}>
            <TD>
              <p className="text-text-primary">{row.label}</p>
              {row.requestedBy && <p className="text-xs text-text-muted">par {row.requestedBy}</p>}
            </TD>
            <TD>
              <Badge tone={row.state.tone} dot>
                {row.state.label}
              </Badge>
            </TD>
            <TD className="whitespace-nowrap text-xs" title={formatDateTime(row.startedAt)}>
              {formatRelativeTime(row.startedAt)}
            </TD>
            <TD className="whitespace-nowrap text-xs">{row.duration}</TD>
            <TD className="max-w-[420px] break-words text-xs">
              {row.result && <p>{row.result}</p>}
              {row.snapshot && <p className="text-xs text-text-muted">instantané {row.snapshot}</p>}
              {row.problem && <p className="text-danger">{row.problem}</p>}
              {row.archiveUrl && (
                <a className="text-accent underline" href={row.archiveUrl} download>
                  Télécharger l&apos;archive
                </a>
              )}
            </TD>
          </TR>
        ))}
      </TBody>
    </Table>
  )
}

function Snapshots({ page }: { page: BackupsPageModel }) {
  const { state, rows } = page.snapshots
  if (state.isLoading) return <LoadingState label="Chargement des instantanés…" rows={3} />
  if (state.isError) return <ErrorState message={state.error.message} onRetry={state.refetch} />
  if (rows.length === 0)
    return (
      <EmptyState
        icon={DatabaseBackup}
        title="Aucun instantané"
        description="La liste est relue après chaque sauvegarde réussie."
      />
    )
  const restore = page.restore
  const diff = page.diff
  return (
    <div className="space-y-3">
      {restore.snapshot && <RestorePanel model={restore} />}
      {diff.visible && (
        <div className="flex flex-wrap items-center gap-3">
          <Button
            size="sm"
            icon={<GitCompare className="h-3.5 w-3.5" />}
            disabled={!diff.canCompare}
            loading={diff.pending}
            onClick={diff.compare}
          >
            Comparer
          </Button>
          <p className="text-xs text-text-muted">
            {diff.selectedCount === 2
              ? 'Deux instantanés choisis.'
              : 'Cochez deux instantanés pour voir ce qui a changé entre eux.'}
          </p>
          {diff.error && (
            <p className="text-xs text-danger" role="alert">
              {diff.error}
            </p>
          )}
        </div>
      )}
      {diff.open && <BackupDiffPanel model={diff} />}
      <Table minWidthClassName="min-w-[760px]">
        <THead>
          <tr>
            {diff.visible && (
              <TH className="w-8">
                <span className="sr-only">Comparer</span>
              </TH>
            )}
            <TH>Instantané</TH>
            <TH>Date</TH>
            <TH>Chemins</TH>
            <TH>Fichiers</TH>
            <TH>Taille</TH>
            <TH>Ajouté</TH>
            {restore.visible && <TH className="text-right">Restaurer</TH>}
          </tr>
        </THead>
        <TBody>
          {rows.map((snapshot) => (
            <TR key={snapshot.id}>
              {diff.visible && (
                <TD>
                  <input
                    type="checkbox"
                    aria-label={`Comparer ${snapshot.shortId}`}
                    checked={diff.isSelected(snapshot)}
                    onChange={() => diff.toggle(snapshot)}
                  />
                </TD>
              )}
              <TD className="font-mono text-xs" title={snapshot.id}>
                {snapshot.shortId}
              </TD>
              <TD className="whitespace-nowrap text-xs">{formatDateTime(snapshot.time)}</TD>
              <TD className="max-w-[320px] truncate text-xs" title={snapshot.paths.join(', ')}>
                {snapshot.paths.join(', ')}
              </TD>
              <TD className="font-mono text-xs">{formatCount(snapshot.totalFiles)}</TD>
              <TD className="font-mono text-xs">{formatBytes(snapshot.totalBytes)}</TD>
              <TD className="font-mono text-xs">{formatBytes(snapshot.dataAdded)}</TD>
              {restore.visible && (
                <TD className="text-right">
                  <Button
                    size="sm"
                    icon={<RotateCcw className="h-3.5 w-3.5" />}
                    onClick={() => restore.choose(snapshot)}
                  >
                    Restaurer…
                  </Button>
                </TD>
              )}
            </TR>
          ))}
        </TBody>
      </Table>
    </div>
  )
}

function RestorePanel({ model }: { model: BackupRestoreModel }) {
  const snapshot = model.snapshot
  if (!snapshot) return null
  const inPlace = model.destination === 'in-place'
  return (
    <div
      className="space-y-3 rounded-lg border border-line bg-card p-4"
      role="group"
      aria-label="Restaurer un instantané"
    >
      <div className="flex items-start justify-between gap-2">
        <div>
          <p className="text-sm font-semibold text-text-primary">
            Restaurer l&apos;instantané <span className="font-mono">{snapshot.shortId}</span> du{' '}
            {formatDateTime(snapshot.time)}
          </p>
          <p className="mt-1 text-xs text-text-secondary">
            {inPlace
              ? "Les fichiers de l'instantané remplacent ceux d'aujourd'hui à leur emplacement d'origine ; les fichiers identiques ne sont pas touchés et rien n'est supprimé. Avant, une sauvegarde de sécurité garde l'état actuel de ces chemins. /etc, /usr, /boot et les données du Manager ne se restaurent jamais en place."
              : "Les fichiers sont restaurés dans un répertoire de quarantaine, /var/lib/monoops-backup/restore/…, accessible à root seulement : rien n'est écrit à leur emplacement d'origine. Copiez ensuite ce qui vous sert. Simulez d'abord pour connaître le volume."}
          </p>
        </div>
        <button
          type="button"
          aria-label="Fermer la restauration"
          onClick={model.close}
          className="rounded-md p-1 text-text-muted hover:bg-elevated hover:text-text-primary"
        >
          <X className="h-4 w-4" />
        </button>
      </div>
      <SegmentedTabs
        options={[
          { value: 'quarantine', label: 'Dans la quarantaine' },
          { value: 'in-place', label: "À l'emplacement d'origine" },
        ]}
        value={model.destination}
        onChange={model.setDestination}
        ariaLabel="Destination de la restauration"
      />
      <FormField
        id="restore-include"
        label={
          inPlace
            ? 'Chemins à remettre en place (un par ligne, obligatoire)'
            : "Chemins à restaurer (un par ligne ; vide pour tout l'instantané)"
        }
        error={model.includeError ?? undefined}
      >
        <textarea
          id="restore-include"
          className="min-h-[72px] w-full rounded-md border border-line bg-surface px-3 py-2 font-mono text-xs text-text-primary outline-none focus:border-accent"
          value={model.include}
          onChange={(event) => model.setInclude(event.target.value)}
        />
      </FormField>
      {model.simulation && (
        <p className="text-xs text-text-secondary" role="status">
          {model.simulation.running
            ? 'Simulation en cours…'
            : model.simulation.current
              ? model.simulation.result
              : 'Les chemins ont changé depuis la simulation : simulez de nouveau.'}
        </p>
      )}
      {inPlace && (
        <label className="flex items-start gap-2 text-xs text-text-secondary">
          <input
            type="checkbox"
            checked={model.acknowledged}
            onChange={(event) => model.setAcknowledged(event.target.checked)}
          />
          Je remplace les fichiers actuels de ces chemins par ceux de l&apos;instantané.
        </label>
      )}
      <div className="flex flex-wrap gap-2">
        <Button
          size="sm"
          loading={model.pending === 'simulate'}
          disabled={model.pending !== null || model.includeError !== null}
          onClick={model.simulate}
        >
          Simuler
        </Button>
        <Button
          size="sm"
          variant={inPlace ? 'danger' : 'primary'}
          loading={model.pending === 'restore'}
          disabled={model.pending !== null || model.includeError !== null || !model.canRestore}
          onClick={model.restore}
        >
          {inPlace ? "Restaurer à l'emplacement d'origine" : 'Restaurer dans la quarantaine'}
        </Button>
      </div>
      {model.started && (
        <p className="text-xs text-text-secondary" role="status">
          {model.started}
        </p>
      )}
      {model.error && (
        <p className="text-xs text-danger" role="alert">
          {model.error}
        </p>
      )}
      <div className="space-y-2 border-t border-line pt-3">
        <FormField
          id="archive-path"
          label="Télécharger une archive .tar.gz d'un chemin de l'instantané"
          error={model.archiveError ?? undefined}
          hint="4 Gio au plus ; l'archive reste disponible 24 heures, jusqu'à la suivante."
        >
          <input
            id="archive-path"
            className={inputClass}
            placeholder="/srv/app"
            value={model.archivePath}
            onChange={(event) => model.setArchivePath(event.target.value)}
          />
        </FormField>
        <Button
          size="sm"
          loading={model.archivePending}
          disabled={
            model.archivePending || model.archivePath.trim() === '' || model.archiveError !== null
          }
          onClick={model.prepareArchive}
        >
          Préparer l&apos;archive
        </Button>
        {model.archive?.url && (
          <p className="text-xs text-text-secondary" role="status">
            {model.archive.description} —{' '}
            <a className="text-accent underline" href={model.archive.url} download>
              Télécharger
            </a>
          </p>
        )}
        {model.archiveFailure && (
          <p className="text-xs text-danger" role="alert">
            {model.archiveFailure}
          </p>
        )}
      </div>
    </div>
  )
}
