import { AlertTriangle, CalendarClock, RefreshCw, SearchX } from 'lucide-react'
import type { ReactNode } from 'react'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/DataTable'
import { EmptyState } from '../../components/ui/EmptyState'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { PageHeader } from '../../components/ui/PageHeader'
import { SearchField } from '../../components/ui/SearchField'
import { SegmentedTabs } from '../../components/ui/SegmentedTabs'
import { formatDateTime, formatRelativeTime } from '../../lib/format'
import { unitStateLabel, unitStateTone } from '../../lib/services'
import { FormField, inputClass } from '../alerts/FormField'
import { useCronPage, type CronPageModel } from './useCronPage'

export function CronPage() {
  const page = useCronPage()
  return (
    <div className="space-y-4">
      <PageHeader
        title="Tâches planifiées"
        breadcrumb={[{ label: 'Système' }, { label: 'Tâches planifiées' }]}
        description="Tâches Cron du système et timers systemd, en lecture seule : créer, modifier ou lancer une tâche Cron reviendrait à exécuter du code en root."
        actions={
          <Button
            size="sm"
            variant="secondary"
            icon={<RefreshCw className="h-3.5 w-3.5" />}
            onClick={page.refresh}
          >
            Actualiser
          </Button>
        }
      />
      {page.state.isLoading && <LoadingState label="Lecture des crontabs…" rows={4} />}
      {page.state.isError && (
        <ErrorState message={page.state.error.message} onRetry={page.state.refetch} />
      )}
      {page.unreadable.length > 0 && (
        <Notice>Fichiers illisibles : {page.unreadable.join(', ')}.</Notice>
      )}
      {page.loaded && (
        <>
          <SegmentedTabs
            options={page.tabs}
            value={page.tab}
            onChange={page.setTab}
            ariaLabel="Sections"
          />
          {page.tab === 'cron' && <CronTab page={page} />}
          {page.tab === 'timers' && <TimersTab page={page} />}
          {page.tab === 'inspector' && <InspectorTab page={page} />}
        </>
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

function CronTab({ page }: { page: CronPageModel }) {
  return (
    <div className="space-y-4">
      {!page.journalAvailable && (
        <Notice>
          Journal système illisible : les dernières exécutions ne peuvent pas être affichées.
        </Notice>
      )}
      <SearchField
        value={page.search}
        onChange={page.setSearch}
        placeholder="Rechercher une commande, un compte, un fichier…"
        ariaLabel="Rechercher une tâche Cron"
        className="max-w-md"
      />
      {page.total === 0 && (
        <EmptyState
          icon={CalendarClock}
          title="Aucune tâche Cron"
          description="/etc/crontab et /etc/cron.d ne contiennent aucune tâche. Les crontabs des comptes ne sont pas lisibles sans privilège."
        />
      )}
      {page.total > 0 && page.entries.length === 0 && (
        <EmptyState
          icon={SearchX}
          title="Aucune tâche ne correspond"
          description="Modifiez la recherche."
        />
      )}
      {page.entries.length > 0 && (
        <Table minWidthClassName="min-w-[900px]">
          <THead>
            <tr>
              <TH>Planification</TH>
              <TH>Compte</TH>
              <TH>Commande</TH>
              <TH>Prochaine</TH>
              <TH>Dernière</TH>
            </tr>
          </THead>
          <TBody>
            {page.entries.map((entry) => (
              <TR key={`${entry.source}:${entry.line}`}>
                <TD className="max-w-[240px]">
                  <p className="text-text-primary">
                    {entry.valid ? entry.description : 'Expression invalide'}
                  </p>
                  <p className="font-mono text-[10px] text-text-muted">
                    {entry.expression} · {entry.source}:{entry.line}
                  </p>
                  {entry.error && <p className="text-[11px] text-danger">{entry.error}</p>}
                </TD>
                <TD className="font-mono text-xs">{entry.user}</TD>
                <TD className="max-w-[360px] break-all font-mono text-[11px]">{entry.command}</TD>
                <TD
                  className="whitespace-nowrap text-xs"
                  title={entry.nextRuns.map((run) => formatDateTime(run)).join('\n')}
                >
                  {entry.nextRuns[0] ? formatDateTime(entry.nextRuns[0]) : '—'}
                </TD>
                <TD className="whitespace-nowrap text-xs" title={formatDateTime(entry.lastRun)}>
                  {entry.lastRun ? formatRelativeTime(entry.lastRun) : '—'}
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      )}
      <p className="font-mono text-[10px] text-text-muted">
        Heures dans le fuseau de la machine ({page.timeZone}). Cron ne journalise pas le code de
        sortie de ses tâches.
      </p>
      {page.directories.length > 0 && (
        <div className="grid gap-3 md:grid-cols-2">
          {page.directories.map((directory) => (
            <Card key={directory.path}>
              <CardHeader title={directory.label} subtitle={directory.path} />
              <CardBody className="space-y-2 text-xs">
                <p className="text-text-secondary">{directory.schedule}</p>
                {directory.nextRun && (
                  <p className="text-text-muted">
                    Prochain passage : {formatDateTime(directory.nextRun)}
                  </p>
                )}
                <p className="font-mono text-[11px] text-text-primary">
                  {directory.scripts.length > 0 ? directory.scripts.join(', ') : 'Aucun script'}
                </p>
              </CardBody>
            </Card>
          ))}
        </div>
      )}
    </div>
  )
}

function TimersTab({ page }: { page: CronPageModel }) {
  if (!page.timersAvailable)
    return (
      <Notice>
        L&apos;inventaire des unités systemd n&apos;est pas encore disponible : voir la page
        Services système.
      </Notice>
    )
  if (page.timers.length === 0)
    return (
      <EmptyState
        icon={CalendarClock}
        title="Aucun timer"
        description="systemd n'a chargé aucun timer."
      />
    )
  return (
    <Table minWidthClassName="min-w-[860px]">
      <THead>
        <tr>
          <TH>Timer</TH>
          <TH>État</TH>
          <TH>Prochaine</TH>
          <TH>Dernière</TH>
          <TH>Service lancé</TH>
          <TH>Dernier résultat</TH>
        </tr>
      </THead>
      <TBody>
        {page.timers.map((timer) => (
          <TR key={timer.name}>
            <TD className="max-w-[260px]">
              <p className="truncate font-mono text-xs text-text-primary">{timer.name}</p>
              <p className="truncate text-[11px] text-text-muted">{timer.description || '—'}</p>
            </TD>
            <TD>
              <Badge tone={unitStateTone(timer.activeState)} dot>
                {unitStateLabel(timer.activeState)}
              </Badge>
            </TD>
            <TD className="whitespace-nowrap text-xs">{formatDateTime(timer.nextElapse)}</TD>
            <TD className="whitespace-nowrap text-xs">{formatRelativeTime(timer.lastTrigger)}</TD>
            <TD className="font-mono text-[11px]">{timer.service ?? '—'}</TD>
            <TD>
              <Badge tone={timer.result.tone}>{timer.result.label}</Badge>
            </TD>
          </TR>
        ))}
      </TBody>
    </Table>
  )
}

function InspectorTab({ page }: { page: CronPageModel }) {
  const { inspector } = page
  return (
    <Card>
      <CardHeader title="Inspecteur d'expression" subtitle="Rien n'est enregistré ni exécuté" />
      <CardBody className="space-y-4">
        <form onSubmit={inspector.onSubmit} noValidate className="flex flex-wrap items-end gap-2">
          <FormField
            id="cron-expression"
            label="Expression Cron (5 champs ou @daily…)"
            error={inspector.errors.expression?.message}
            className="min-w-[240px] flex-1"
          >
            <input
              id="cron-expression"
              autoComplete="off"
              className={`${inputClass} font-mono`}
              {...inspector.register('expression')}
            />
          </FormField>
          <Button type="submit" size="sm" variant="primary" loading={inspector.state.isLoading}>
            Calculer
          </Button>
        </form>
        {inspector.state.isError && (
          <p className="text-xs text-danger" role="alert">
            {inspector.state.error.message}
          </p>
        )}
        {inspector.result && (
          <div className="space-y-2">
            <p className="text-sm font-semibold text-text-primary">
              {inspector.result.description}
            </p>
            {inspector.result.nextRuns.length === 0 ? (
              <p className="text-xs text-text-muted">
                Aucune exécution dans les cinq prochaines années.
              </p>
            ) : (
              <ol className="space-y-0.5 font-mono text-xs">
                {inspector.result.nextRuns.map((run) => (
                  <li key={run}>{formatDateTime(run)}</li>
                ))}
              </ol>
            )}
            <p className="text-[11px] text-text-muted">
              Heures dans le fuseau de la machine ({page.timeZone}).
            </p>
          </div>
        )}
      </CardBody>
    </Card>
  )
}
