import {
  AlertTriangle,
  Download,
  FileSearch,
  Pause,
  Play,
  RefreshCw,
  ScrollText,
  Search,
  X,
} from 'lucide-react'
import { Chart } from '../../components/charts/Chart'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/DataTable'
import { EmptyState } from '../../components/ui/EmptyState'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { PageHeader } from '../../components/ui/PageHeader'
import { Select } from '../../components/ui/Select'
import { cn } from '../../lib/cn'
import { formatBytes, formatCount } from '../../lib/format'
import { useJournalPage, type JournalPageModel } from './useJournalPage'

const labelClass = 'mb-1.5 block text-xs text-text-secondary'
const inputClass =
  'h-9 w-full rounded-md border border-line bg-surface px-3 text-sm text-text-primary outline-none focus:border-accent/60'
const linkClass =
  'inline-flex h-8 items-center gap-1.5 rounded-md border border-line bg-card px-2.5 text-xs text-text-secondary hover:bg-elevated hover:text-text-primary'

export function JournalPage() {
  const page = useJournalPage()

  return (
    <div className="space-y-4">
      <PageHeader
        title="Journaux système"
        breadcrumb={[{ label: 'Système' }, { label: 'Journaux système' }]}
        badge={
          page.summary.diskUsageBytes !== null && (
            <Badge tone="neutral">{formatBytes(page.summary.diskUsageBytes)} sur disque</Badge>
          )
        }
        description="Journal systemd de la machine, en lecture seule. Les champs sensibles (ligne de commande, mots de passe, jetons, secrets) sont masqués."
        actions={
          <div className="flex flex-wrap gap-2">
            <Button
              size="sm"
              variant={page.follow ? 'primary' : 'secondary'}
              icon={
                page.follow ? <Pause className="h-3.5 w-3.5" /> : <Play className="h-3.5 w-3.5" />
              }
              onClick={page.toggleFollow}
              aria-pressed={page.follow}
            >
              {page.follow ? 'Suivi en direct' : 'Suivre en direct'}
            </Button>
            <Button
              size="sm"
              variant="secondary"
              icon={<RefreshCw className="h-3.5 w-3.5" />}
              onClick={page.refresh}
            >
              Actualiser
            </Button>
          </div>
        }
      />

      {page.summary.limitedAccess && (
        <div
          className="flex items-start gap-2 rounded-md border border-warning/40 bg-warning/5 px-3 py-2.5 text-xs text-text-secondary"
          role="status"
        >
          <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-warning" aria-hidden="true" />
          <p>
            Le Manager ne lit que ses propres entrées : ajoutez le compte <code>monoops</code> au
            groupe <code>systemd-journal</code> puis redémarrez le service (voir le README).
          </p>
        </div>
      )}

      <SummaryCard page={page} />
      <Filters page={page} />

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_380px]">
        <Entries page={page} />
        {page.selected && <Inspector page={page} />}
      </div>
    </div>
  )
}

function SummaryCard({ page }: { page: JournalPageModel }) {
  const { summary } = page
  return (
    <Card>
      <CardHeader
        title="Dernières 24 heures"
        subtitle="Entrées par priorité, tranches de 15 minutes"
        icon={<ScrollText className="h-3.5 w-3.5" />}
        actions={
          summary.total !== null && (
            <Badge tone="neutral">{formatCount(summary.total)} entrées</Badge>
          )
        }
      />
      <CardBody className="space-y-3 p-4">
        {summary.state.isLoading && <LoadingState label="Chargement de la synthèse…" rows={2} />}
        {summary.state.isError && (
          <ErrorState message={summary.state.error.message} onRetry={summary.state.refetch} />
        )}
        {summary.chartOption && (
          <>
            <Chart
              option={summary.chartOption}
              height={180}
              label="Volume du journal sur 24 heures"
            />
            <dl className="grid grid-cols-2 gap-2 sm:grid-cols-4">
              {summary.totals.map((total) => (
                <div key={total.label} className="rounded-md border border-line/70 px-3 py-2">
                  <dt className="truncate text-xs text-text-muted">{total.label}</dt>
                  <dd className="font-mono text-sm text-text-primary">
                    {formatCount(total.total)}
                  </dd>
                </div>
              ))}
            </dl>
            {summary.truncated && (
              <p className="text-xs text-text-muted">
                Plus de 200 000 entrées : seules les plus récentes sont comptées.
              </p>
            )}
          </>
        )}
      </CardBody>
    </Card>
  )
}

function Filters({ page }: { page: JournalPageModel }) {
  return (
    <Card>
      <CardBody className="space-y-3 p-3">
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
          <div className="min-w-0">
            <label htmlFor="journalSource" className={labelClass}>
              Source
            </label>
            <Select
              id="journalSource"
              containerClassName="w-full"
              value={page.source}
              onChange={(event) => page.setSource(event.target.value)}
            >
              {page.sourceOptions.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </Select>
          </div>
          <div className="min-w-0">
            <label htmlFor="journalPriority" className={labelClass}>
              Priorité
            </label>
            <Select
              id="journalPriority"
              containerClassName="w-full"
              value={page.priority}
              onChange={(event) => page.setPriority(event.target.value)}
            >
              {page.priorityOptions.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </Select>
          </div>
          <div className="min-w-0">
            <label htmlFor="journalUnit" className={labelClass}>
              Unité
            </label>
            <Select
              id="journalUnit"
              containerClassName="w-full"
              value={page.unitDisabled ? '' : page.unit}
              disabled={page.unitDisabled}
              onChange={(event) => page.setUnit(event.target.value)}
            >
              <option value="">Toutes les unités</option>
              {page.unitOptions.map((unit) => (
                <option key={unit} value={unit}>
                  {unit}
                </option>
              ))}
            </Select>
          </div>
          <div className="min-w-0">
            <label htmlFor="journalBoot" className={labelClass}>
              Démarrage
            </label>
            <Select
              id="journalBoot"
              containerClassName="w-full"
              value={page.boot}
              onChange={(event) => page.setBoot(event.target.value)}
            >
              <option value="">Tous les démarrages</option>
              {page.bootOptions.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </Select>
          </div>
          <div className="min-w-0">
            <label htmlFor="journalPeriod" className={labelClass}>
              Période
            </label>
            <Select
              id="journalPeriod"
              containerClassName="w-full"
              value={page.period}
              onChange={(event) => page.setPeriod(event.target.value)}
            >
              {page.periodOptions.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </Select>
          </div>
        </div>
        <form
          className="flex flex-wrap items-start gap-3"
          onSubmit={page.onSearch}
          noValidate
          role="search"
        >
          <div className="min-w-[200px] flex-1">
            <label htmlFor="journalSearch" className={labelClass}>
              Texte recherché
            </label>
            <input
              id="journalSearch"
              type="search"
              autoComplete="off"
              placeholder="Message, unité ou identifiant"
              className={inputClass}
              {...page.register('q')}
            />
            {page.errors.q && (
              <p className="mt-1 text-xs text-danger" role="alert">
                {page.errors.q.message}
              </p>
            )}
          </div>
          {page.customRange && (
            <>
              <div>
                <label htmlFor="journalFrom" className={labelClass}>
                  Début
                </label>
                <input
                  id="journalFrom"
                  type="datetime-local"
                  className={inputClass}
                  {...page.register('from')}
                />
              </div>
              <div>
                <label htmlFor="journalTo" className={labelClass}>
                  Fin (exclue)
                </label>
                <input
                  id="journalTo"
                  type="datetime-local"
                  className={inputClass}
                  {...page.register('to')}
                />
                {page.errors.to && (
                  <p className="mt-1 text-xs text-danger" role="alert">
                    {page.errors.to.message}
                  </p>
                )}
              </div>
            </>
          )}
          <div className="flex flex-wrap items-center gap-2 self-end">
            <Button
              type="submit"
              size="sm"
              variant="primary"
              icon={<Search className="h-3.5 w-3.5" />}
            >
              Appliquer
            </Button>
            <a href={page.exportLogUrl} download className={linkClass}>
              <Download className="h-3.5 w-3.5" aria-hidden="true" />
              .log
            </a>
            <a href={page.exportJsonUrl} download className={linkClass}>
              <Download className="h-3.5 w-3.5" aria-hidden="true" />
              .json
            </a>
          </div>
        </form>
      </CardBody>
    </Card>
  )
}

function Entries({ page }: { page: JournalPageModel }) {
  const { entries } = page
  return (
    <Card className="min-w-0">
      <CardHeader
        title="Entrées"
        subtitle="De la plus récente à la plus ancienne"
        icon={<FileSearch className="h-3.5 w-3.5" />}
        actions={
          page.follow && (
            <Badge tone="success" dot>
              Actualisé toutes les 2 s
            </Badge>
          )
        }
      />
      {page.followError && (
        <p className="border-b border-line/70 px-4 py-2 text-xs text-danger" role="alert">
          Suivi interrompu : {page.followError}
        </p>
      )}
      {entries.state.isLoading && <LoadingState label="Lecture du journal…" rows={6} />}
      {entries.state.isError && (
        <div className="p-4">
          <ErrorState message={entries.state.error.message} onRetry={entries.state.refetch} />
        </div>
      )}
      {!entries.state.isLoading && !entries.state.isError && entries.rows.length === 0 && (
        <EmptyState
          icon={FileSearch}
          title="Aucune entrée"
          description={
            entries.scanLimitReached
              ? 'Aucune correspondance dans les entrées examinées : continuez la recherche plus loin.'
              : 'Aucune entrée ne correspond à ces filtres.'
          }
        />
      )}
      {entries.rows.length > 0 && (
        <Table minWidthClassName="min-w-[720px]">
          <THead>
            <TR>
              <TH className="w-36">Date</TH>
              <TH className="w-32">Priorité</TH>
              <TH className="w-44">Source</TH>
              <TH>Message</TH>
            </TR>
          </THead>
          <TBody>
            {entries.rows.map((row) => (
              <TR
                key={row.cursor}
                className={cn(row.cursor === page.selected && 'bg-elevated')}
                aria-selected={row.cursor === page.selected}
              >
                <TD className="whitespace-nowrap font-mono text-xs">
                  <button
                    type="button"
                    className="text-left text-accent underline-offset-2 hover:underline"
                    onClick={() => page.select(row.cursor)}
                    aria-label={`Inspecter l'entrée du ${row.timeLabel}`}
                  >
                    <time dateTime={row.time}>{row.timeLabel}</time>
                  </button>
                </TD>
                <TD>
                  <Badge tone={row.priorityTone}>{row.priorityLabel}</Badge>
                </TD>
                <TD className="max-w-[11rem] truncate font-mono text-xs" title={row.identifier}>
                  {row.source}
                </TD>
                <TD className="whitespace-pre-wrap break-words font-mono text-xs leading-relaxed">
                  {row.message}
                  {row.truncated && <span className="text-text-muted"> … (tronqué)</span>}
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      )}
      {(entries.hasOlder || entries.scanLimitReached || entries.skipped > 0) && (
        <div className="flex flex-wrap items-center gap-3 border-t border-line/70 px-4 py-3">
          {entries.hasOlder && (
            <Button
              size="sm"
              variant="secondary"
              loading={entries.isLoadingOlder}
              onClick={entries.loadOlder}
            >
              {entries.scanLimitReached
                ? 'Continuer la recherche'
                : 'Charger les entrées plus anciennes'}
            </Button>
          )}
          {entries.scanLimitReached && (
            <p className="text-xs text-text-muted">
              Recherche arrêtée après 5 000 entrées examinées.
            </p>
          )}
          {entries.skipped > 0 && (
            <p className="text-xs text-text-muted">
              {formatCount(entries.skipped)} entrée(s) trop volumineuse(s) ignorée(s).
            </p>
          )}
        </div>
      )}
    </Card>
  )
}

function Inspector({ page }: { page: JournalPageModel }) {
  const { inspector } = page
  return (
    <Card className="order-first min-w-0 self-start xl:sticky xl:top-4 xl:order-none">
      <CardHeader
        title="Inspecteur"
        subtitle={inspector.entry ? inspector.entry.timeLabel : 'Entrée sélectionnée'}
        icon={<FileSearch className="h-3.5 w-3.5" />}
        actions={
          <Button
            size="sm"
            variant="ghost"
            icon={<X className="h-3.5 w-3.5" />}
            onClick={page.closeInspector}
            aria-label="Fermer l'inspecteur"
          />
        }
      />
      <CardBody className="space-y-3 p-4">
        {inspector.state.isLoading && <LoadingState label="Lecture de l'entrée…" rows={4} />}
        {inspector.state.isError && (
          <ErrorState message={inspector.state.error.message} onRetry={inspector.state.refetch} />
        )}
        {inspector.entry && (
          <>
            <div className="flex flex-wrap items-center gap-2">
              <Badge tone={inspector.entry.priorityTone}>{inspector.entry.priorityLabel}</Badge>
              <span className="font-mono text-xs text-text-secondary">
                {inspector.entry.source}
              </span>
            </div>
            <dl className="max-h-[420px] space-y-1.5 overflow-y-auto">
              {inspector.fields.map((field) => (
                <div
                  key={field.name}
                  className="grid grid-cols-[minmax(0,9rem)_minmax(0,1fr)] gap-2"
                >
                  <dt className="truncate text-xs text-text-muted" title={field.name}>
                    {field.name}
                  </dt>
                  <dd
                    className={cn(
                      'whitespace-pre-wrap break-words text-xs',
                      field.masked ? 'italic text-warning' : 'text-text-primary',
                    )}
                  >
                    {field.value}
                  </dd>
                </div>
              ))}
            </dl>
            <details>
              <summary className="cursor-pointer text-xs text-text-secondary">JSON brut</summary>
              <pre className="mt-2 max-h-64 overflow-auto rounded-md border border-line/70 bg-surface p-2 text-xs text-text-primary">
                {inspector.json}
              </pre>
            </details>
          </>
        )}
      </CardBody>
    </Card>
  )
}
