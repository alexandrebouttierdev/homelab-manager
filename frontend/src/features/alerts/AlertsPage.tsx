import { BellOff, CheckCircle2, History, RefreshCw, VolumeX } from 'lucide-react'
import { Link } from 'react-router-dom'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/DataTable'
import { EmptyState } from '../../components/ui/EmptyState'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { PageHeader } from '../../components/ui/PageHeader'
import { SegmentedTabs } from '../../components/ui/SegmentedTabs'
import type { AlertRow } from '../../lib/alerts'
import { cn } from '../../lib/cn'
import { formatCount, formatDateTime, formatRelativeTime } from '../../lib/format'
import { AlertRulesPanel } from './AlertRulesPanel'
import { ChannelsPanel } from './ChannelsPanel'
import { SilenceForm } from './SilenceForm'
import { SilencesPanel } from './SilencesPanel'
import { useAlertsPage, useAlertsTabs, type AlertsPageModel } from './useAlertsPage'

export function AlertsPage() {
  const { tab, setTab, options } = useAlertsTabs()

  return (
    <div className="space-y-4">
      <PageHeader
        title="Alertes"
        breadcrumb={[{ label: 'Supervision' }, { label: 'Alertes' }]}
        description="Règles évaluées toutes les 15 secondes sur les données déjà collectées, notifiées aux canaux configurés."
      />
      <SegmentedTabs options={options} value={tab} onChange={setTab} ariaLabel="Sections" />
      {tab === 'alerts' && <AlertsPanel />}
      {tab === 'rules' && <AlertRulesPanel />}
      {tab === 'silences' && <SilencesPanel />}
      {tab === 'channels' && <ChannelsPanel />}
    </div>
  )
}

function AlertsPanel() {
  const page = useAlertsPage()

  return (
    <div className="space-y-4">
      {page.summary && (
        <div className="grid grid-cols-2 gap-3 xl:grid-cols-5">
          <Counter
            label="Critiques"
            value={formatCount(page.summary.critical)}
            tone={page.summary.critical > 0 ? 'text-danger' : 'text-text-primary'}
          />
          <Counter
            label="Avertissements"
            value={formatCount(page.summary.warning)}
            tone={page.summary.warning > 0 ? 'text-warning' : 'text-text-primary'}
          />
          <Counter label="Informations" value={formatCount(page.summary.info)} tone="text-info" />
          <Counter
            label="Résolues sur 24 h"
            value={formatCount(page.summary.resolvedLast24h)}
            tone="text-text-primary"
          />
          <Counter
            label="Résolution moyenne (24 h)"
            value={page.summary.meanResolution}
            tone="text-text-primary"
          />
        </div>
      )}

      <div className="flex flex-wrap items-center justify-between gap-2">
        <SegmentedTabs
          options={page.viewOptions}
          value={page.view}
          onChange={page.setView}
          ariaLabel="Alertes affichées"
        />
        <Button
          size="sm"
          variant="secondary"
          icon={<RefreshCw className="h-3.5 w-3.5" />}
          onClick={page.refresh}
        >
          Actualiser
        </Button>
      </div>

      {page.acknowledgeError && (
        <p className="text-xs text-danger" role="alert">
          {page.acknowledgeError}
        </p>
      )}

      {page.silence.isOpen && <SilenceForm model={page.silence} />}

      {page.view === 'open' ? <OpenAlerts page={page} /> : <AlertHistory page={page} />}
    </div>
  )
}

function Counter({ label, value, tone }: { label: string; value: string; tone: string }) {
  return (
    <div className="rounded-lg border border-line bg-card p-4">
      <p className="font-mono text-[11px] uppercase tracking-wider text-text-muted">{label}</p>
      <p className={cn('mt-1 font-mono text-2xl font-semibold tracking-tight', tone)}>{value}</p>
    </div>
  )
}

function OpenAlerts({ page }: { page: AlertsPageModel }) {
  const { state, rows } = page.open
  if (state.isLoading) return <LoadingState label="Chargement des alertes…" rows={3} />
  if (state.isError) return <ErrorState message={state.error.message} onRetry={state.refetch} />
  if (rows.length === 0)
    return (
      <EmptyState
        icon={CheckCircle2}
        title="Aucune alerte ouverte"
        description="Toutes les règles actives sont respectées, ou leurs données ne sont pas encore disponibles."
      />
    )
  return (
    <Table>
      <THead>
        <TR>
          <TH>Sévérité</TH>
          <TH>Règle</TH>
          <TH>Cible</TH>
          <TH>Valeur</TH>
          <TH>Depuis</TH>
          <TH>État</TH>
          <TH className="relative">
            <span className="sr-only">Actions</span>
          </TH>
        </TR>
      </THead>
      <TBody>
        {rows.map((row) => (
          <TR key={row.id}>
            <TD>
              <Badge tone={row.severity.tone} dot>
                {row.severity.label}
              </Badge>
            </TD>
            <TD className="text-text-primary">{row.ruleName}</TD>
            <TD>
              <Target row={row} />
            </TD>
            <TD className="font-mono text-xs">{row.value}</TD>
            <TD className="whitespace-nowrap text-xs" title={formatDateTime(row.firedAt)}>
              {row.duration}
            </TD>
            <TD>
              <Badge tone={row.state.tone}>{row.state.label}</Badge>
              {row.acknowledgedBy && (
                <span className="ml-2 text-[11px] text-text-muted">par {row.acknowledgedBy}</span>
              )}
              {row.silenced && (
                <Badge tone="neutral" className="ml-2">
                  Sous silence
                </Badge>
              )}
            </TD>
            <TD className="whitespace-nowrap text-right">
              <div className="flex justify-end gap-1">
                {page.canAcknowledge && row.acknowledgeable && (
                  <Button
                    size="sm"
                    icon={<BellOff className="h-3.5 w-3.5" />}
                    loading={page.acknowledgingId === row.id}
                    onClick={() => page.acknowledge(row.id)}
                  >
                    Acquitter
                  </Button>
                )}
                {page.canSilence && row.silenceable && (
                  <Button
                    size="sm"
                    icon={<VolumeX className="h-3.5 w-3.5" />}
                    onClick={() => page.startSilence(row.id)}
                  >
                    Silence
                  </Button>
                )}
              </div>
            </TD>
          </TR>
        ))}
      </TBody>
    </Table>
  )
}

function AlertHistory({ page }: { page: AlertsPageModel }) {
  const { state, rows } = page.history
  if (state.isLoading) return <LoadingState label="Chargement de l'historique…" rows={3} />
  if (state.isError) return <ErrorState message={state.error.message} onRetry={state.refetch} />
  if (rows.length === 0)
    return (
      <EmptyState
        icon={History}
        title="Aucune alerte résolue"
        description="Les alertes résolues restent consultables pendant la durée de rétention des métriques."
      />
    )
  return (
    <Table>
      <THead>
        <TR>
          <TH>Sévérité</TH>
          <TH>Règle</TH>
          <TH>Cible</TH>
          <TH>Valeur</TH>
          <TH>Déclenchée</TH>
          <TH>Durée</TH>
          <TH>Résolution</TH>
        </TR>
      </THead>
      <TBody>
        {rows.map((row) => (
          <TR key={row.id}>
            <TD>
              <Badge tone={row.severity.tone} dot>
                {row.severity.label}
              </Badge>
            </TD>
            <TD className="text-text-primary">{row.ruleName}</TD>
            <TD>
              <Target row={row} />
            </TD>
            <TD className="font-mono text-xs">{row.value}</TD>
            <TD className="whitespace-nowrap text-xs" title={formatDateTime(row.firedAt)}>
              {formatRelativeTime(row.firedAt)}
            </TD>
            <TD className="whitespace-nowrap text-xs">{row.duration}</TD>
            <TD className="text-xs">{row.resolution}</TD>
          </TR>
        ))}
      </TBody>
    </Table>
  )
}

function Target({ row }: { row: AlertRow }) {
  return (
    <div className="space-y-0.5">
      <p className="text-text-primary">{row.target}</p>
      {row.link && (
        <Link to={row.link.to} className="text-[11px] text-accent hover:underline">
          {row.link.label}
        </Link>
      )}
    </div>
  )
}
