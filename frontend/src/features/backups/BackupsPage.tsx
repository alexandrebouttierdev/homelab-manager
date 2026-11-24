import { AlertTriangle, DatabaseBackup, History, Play, RefreshCw } from 'lucide-react'
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
import { BackupConfigForm } from './BackupConfigForm'
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
        description="Sauvegardes chiffrées par restic, exécutées par l'unité dédiée homelab-backup@.service."
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
      {page.tab === 'config' && <BackupConfigForm model={page.configForm} />}
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
      <p className="font-mono text-[11px] uppercase tracking-wider text-text-muted">{label}</p>
      <p className="mt-1 truncate text-sm font-semibold text-text-primary">{value}</p>
      <p className="mt-0.5 truncate text-[11px] text-text-muted" title={detail}>
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
              {row.requestedBy && (
                <p className="text-[11px] text-text-muted">par {row.requestedBy}</p>
              )}
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
            <TD className="text-xs">
              {row.result && <p>{row.result}</p>}
              {row.snapshot && (
                <p className="font-mono text-[10px] text-text-muted">instantané {row.snapshot}</p>
              )}
              {row.problem && <p className="text-danger">{row.problem}</p>}
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
  return (
    <Table minWidthClassName="min-w-[760px]">
      <THead>
        <tr>
          <TH>Instantané</TH>
          <TH>Date</TH>
          <TH>Chemins</TH>
          <TH>Fichiers</TH>
          <TH>Taille</TH>
          <TH>Ajouté</TH>
        </tr>
      </THead>
      <TBody>
        {rows.map((snapshot) => (
          <TR key={snapshot.id}>
            <TD className="font-mono text-xs" title={snapshot.id}>
              {snapshot.shortId}
            </TD>
            <TD className="whitespace-nowrap text-xs">{formatDateTime(snapshot.time)}</TD>
            <TD
              className="max-w-[320px] truncate font-mono text-[11px]"
              title={snapshot.paths.join(', ')}
            >
              {snapshot.paths.join(', ')}
            </TD>
            <TD className="font-mono text-xs">{formatCount(snapshot.totalFiles)}</TD>
            <TD className="font-mono text-xs">{formatBytes(snapshot.totalBytes)}</TD>
            <TD className="font-mono text-xs">{formatBytes(snapshot.dataAdded)}</TD>
          </TR>
        ))}
      </TBody>
    </Table>
  )
}
