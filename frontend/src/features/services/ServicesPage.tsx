import {
  AlertTriangle,
  CheckCircle2,
  CircleSlash,
  Clock,
  RefreshCw,
  SearchX,
  Wrench,
  X,
} from 'lucide-react'
import type { ReactNode } from 'react'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/DataTable'
import { EmptyState } from '../../components/ui/EmptyState'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { PageHeader } from '../../components/ui/PageHeader'
import { RevealOnMount } from '../../components/ui/RevealOnMount'
import { SearchField } from '../../components/ui/SearchField'
import { SegmentedTabs } from '../../components/ui/SegmentedTabs'
import { Select } from '../../components/ui/Select'
import { cn } from '../../lib/cn'
import { formatBytes, formatCount, formatDateTime, formatRelativeTime } from '../../lib/format'
import { displayMessage, formatJournalTime } from '../../lib/journal'
import {
  formatCpuTime,
  formatUnitLimit,
  unitFileStateLabel,
  unitStateLabel,
  unitStateTone,
  unitTypeLabel,
} from '../../lib/services'
import { UnitActionButtons, UnitActionStatus } from './UnitActions'
import { useServicesPage, type ServicesPageModel } from './useServicesPage'

export function ServicesPage() {
  const page = useServicesPage()
  const { inventory } = page

  return (
    <div className="space-y-4">
      <PageHeader
        title="Services système"
        breadcrumb={[{ label: 'Système' }, { label: 'Services système' }]}
        badge={
          inventory.status && <Badge tone={inventory.status.tone}>{inventory.status.label}</Badge>
        }
        description="Services, timers et sockets systemd de la machine, relus toutes les 30 secondes. Seules les unités listées dans la configuration du Manager peuvent être démarrées, arrêtées, redémarrées ou rechargées."
        actions={
          <Button
            size="sm"
            variant="secondary"
            icon={<RefreshCw className="h-3.5 w-3.5" />}
            onClick={inventory.refresh}
          >
            Actualiser
          </Button>
        }
      />

      {inventory.state.isLoading && <LoadingState label="Chargement des unités…" rows={4} />}
      {inventory.state.isError && (
        <ErrorState message={inventory.state.error.message} onRetry={inventory.state.refetch} />
      )}

      {inventory.unavailable && (
        <Notice>
          systemd n&apos;est pas joignable sur le bus système de la machine : l&apos;inventaire des
          services est indisponible. Le reste du Manager fonctionne.
        </Notice>
      )}
      {inventory.outdated && (
        <Notice>
          La dernière collecte a échoué : l&apos;inventaire affiché date du{' '}
          {formatDateTime(inventory.collectedAt)}.
        </Notice>
      )}

      {inventory.counts && (
        <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
          <Counter label="Unités" value={inventory.counts.total} tone="text-text-primary" />
          <Counter label="Actives" value={inventory.counts.active} tone="text-success" />
          <Counter
            label="En échec"
            value={inventory.counts.failed}
            tone={inventory.counts.failed > 0 ? 'text-danger' : 'text-text-primary'}
          />
          <Counter label="Timers actifs" value={inventory.counts.activeTimers} tone="text-info" />
        </div>
      )}

      {inventory.loaded && inventory.total > 0 && <Filters page={page} />}

      {inventory.loaded && inventory.total === 0 && !inventory.unavailable && (
        <EmptyState
          icon={Clock}
          title="Aucune unité pour l'instant"
          description="La première collecte n'est pas terminée, ou systemd n'a renvoyé aucun service, timer ni socket."
        />
      )}

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_420px]">
        <div className="min-w-0 space-y-4">
          <Units page={page} />
        </div>
        {page.selectedName && (
          <RevealOnMount key={page.selectedName}>
            <Detail page={page} />
          </RevealOnMount>
        )}
      </div>
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

function Counter({ label, value, tone }: { label: string; value: number; tone: string }) {
  return (
    <div className="rounded-lg border border-line bg-card p-4">
      <p className="font-mono text-[11px] uppercase tracking-wider text-text-muted">{label}</p>
      <p className={cn('mt-1 font-mono text-2xl font-semibold tracking-tight', tone)}>
        {formatCount(value)}
      </p>
    </div>
  )
}

function Filters({ page }: { page: ServicesPageModel }) {
  const { filters } = page
  return (
    <div className="flex flex-wrap items-center gap-2 rounded-lg border border-line bg-card p-2.5">
      <SearchField
        value={filters.search}
        onChange={filters.setSearch}
        placeholder="Rechercher une unité…"
        ariaLabel="Rechercher une unité par son nom"
        className="min-w-[220px] flex-1 border-0 bg-surface"
      />
      <SegmentedTabs
        ariaLabel="Filtrer par état"
        value={filters.state}
        onChange={filters.setState}
        options={filters.stateOptions}
      />
      <Select
        aria-label="Filtrer par type"
        value={filters.type}
        onChange={(event) => filters.setType(event.target.value)}
        className="min-w-[150px]"
      >
        {filters.typeOptions.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </Select>
    </div>
  )
}

function Units({ page }: { page: ServicesPageModel }) {
  if (!page.inventory.loaded || page.inventory.total === 0) return null
  if (page.units.length === 0) {
    return (
      <EmptyState
        icon={SearchX}
        title="Aucune unité ne correspond"
        description="Modifiez la recherche, l'état ou le type."
        action={
          <Button size="sm" variant="secondary" onClick={page.filters.reset}>
            Réinitialiser les filtres
          </Button>
        }
      />
    )
  }
  return (
    <>
      <Table minWidthClassName="min-w-[860px]">
        <THead>
          <tr>
            <TH>Unité</TH>
            <TH>Type</TH>
            <TH>État</TH>
            <TH>Démarrage</TH>
            <TH>PID</TH>
            <TH>Mémoire</TH>
            <TH>CPU</TH>
            <TH>Dernier changement</TH>
          </tr>
        </THead>
        <TBody>
          {page.units.map((unit) => (
            <TR
              key={unit.name}
              className={cn(unit.name === page.selectedName && 'bg-elevated/60')}
              aria-selected={unit.name === page.selectedName}
            >
              <TD className="max-w-[280px]">
                <button
                  type="button"
                  onClick={() => page.select(unit.name)}
                  className="block w-full min-w-0 text-left"
                >
                  <span className="block truncate font-mono text-xs text-accent hover:underline">
                    {unit.name}
                  </span>
                  <span className="block truncate text-[11px] text-text-muted">
                    {unit.description || '—'}
                  </span>
                </button>
              </TD>
              <TD className="text-xs">{unitTypeLabel(unit.type)}</TD>
              <TD>
                <Badge tone={unitStateTone(unit.activeState)} dot>
                  {unitStateLabel(unit.activeState)}
                </Badge>
                <span className="ml-1.5 font-mono text-[10px] text-text-muted">
                  {unit.subState}
                </span>
              </TD>
              <TD className="whitespace-nowrap text-xs">
                {unitFileStateLabel(unit.unitFileState)}
              </TD>
              <TD className="whitespace-nowrap font-mono text-xs">{unit.mainPid ?? '—'}</TD>
              <TD className="whitespace-nowrap font-mono text-xs">
                {formatBytes(unit.memoryBytes)}
              </TD>
              <TD className="whitespace-nowrap font-mono text-xs">
                {formatCpuTime(unit.cpuNanoseconds)}
              </TD>
              <TD className="whitespace-nowrap font-mono text-[10px]">
                {formatRelativeTime(unit.stateChangedAt)}
              </TD>
            </TR>
          ))}
        </TBody>
      </Table>
      <p className="font-mono text-[10px] text-text-muted">
        {page.units.length} sur {page.inventory.total} unité(s) · en échec d&apos;abord · collecte
        du {formatDateTime(page.inventory.collectedAt)}
      </p>
    </>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[120px_minmax(0,1fr)] gap-2 py-1">
      <dt className="text-[11px] text-text-muted">{label}</dt>
      <dd className="min-w-0 break-words font-mono text-xs text-text-primary">{children}</dd>
    </div>
  )
}

function NameList({ names }: { names: string[] }) {
  if (names.length === 0) return <>—</>
  return (
    <ul className="flex flex-wrap gap-1">
      {names.map((name) => (
        <li key={name} className="rounded border border-line/70 px-1.5 py-0.5 text-[10px]">
          {name}
        </li>
      ))}
    </ul>
  )
}

function Detail({ page }: { page: ServicesPageModel }) {
  const { detail } = page
  const data = detail.data
  return (
    <Card className="h-fit xl:sticky xl:top-4">
      <CardHeader
        title={<span className="font-mono">{page.selectedName}</span>}
        subtitle="Détail de l'unité"
        icon={<Wrench className="h-3.5 w-3.5" />}
        actions={
          <button
            type="button"
            aria-label="Fermer le détail"
            onClick={() => page.select(null)}
            className="rounded-md p-1 text-text-muted hover:bg-elevated hover:text-text-primary"
          >
            <X className="h-4 w-4" />
          </button>
        }
      />
      <CardBody className="space-y-4 p-4">
        {detail.state.isLoading && <LoadingState label="Lecture de l'unité…" rows={3} />}
        {detail.state.isError && (
          <ErrorState message={detail.state.error.message} onRetry={detail.state.refetch} />
        )}
        {data && (
          <>
            {(detail.actions.length > 0 || page.unitAction.progress) && (
              <section className="space-y-2">
                <h3 className="text-xs font-semibold text-text-primary">Actions</h3>
                <UnitActionButtons
                  model={page.unitAction}
                  choices={detail.actions}
                  unitName={data.unit.name}
                />
                <UnitActionStatus model={page.unitAction} />
              </section>
            )}
            <section>
              <h3 className="mb-1 text-xs font-semibold text-text-primary">État</h3>
              <dl>
                <Field label="Description">{data.unit.description || '—'}</Field>
                <Field label="État">
                  <Badge tone={unitStateTone(data.unit.activeState)} dot>
                    {unitStateLabel(data.unit.activeState)}
                  </Badge>{' '}
                  {data.unit.subState}
                </Field>
                <Field label="Chargement">{data.unit.loadState || '—'}</Field>
                <Field label="Démarrage">{unitFileStateLabel(data.unit.unitFileState)}</Field>
                <Field label="Résultat">{data.unit.result ?? '—'}</Field>
                <Field label="Depuis">{formatDateTime(data.unit.stateChangedAt)}</Field>
                {data.unit.type === 'timer' && (
                  <>
                    <Field label="Prochaine">{formatDateTime(data.unit.nextElapse)}</Field>
                    <Field label="Dernière">{formatDateTime(data.unit.lastTrigger)}</Field>
                  </>
                )}
                {data.unit.type !== 'service' && (
                  <Field label="Déclenche">
                    <NameList names={data.unit.triggers} />
                  </Field>
                )}
              </dl>
            </section>

            <section>
              <h3 className="mb-1 text-xs font-semibold text-text-primary">Définition</h3>
              <dl>
                <Field label="Fichier">{data.definition.fragmentPath ?? '—'}</Field>
                <Field label="ExecStart">
                  {data.definition.execStart.length === 0
                    ? '—'
                    : data.definition.execStart.map((line) => (
                        <pre
                          key={line}
                          className="whitespace-pre-wrap break-all rounded bg-surface px-2 py-1 text-[11px]"
                        >
                          {line}
                        </pre>
                      ))}
                </Field>
                <Field label="Restart">{data.definition.restart ?? '—'}</Field>
                <Field label="Slice">{data.definition.slice ?? '—'}</Field>
                <Field label="TasksMax">{formatUnitLimit(data.definition.tasksMax)}</Field>
                <Field label="LimitNOFILE">{formatUnitLimit(data.definition.limitNofile)}</Field>
                <Field label="MemoryMax">
                  {formatUnitLimit(data.definition.memoryMax, 'bytes')}
                </Field>
                <Field label="Requires">
                  <NameList names={data.definition.requires} />
                </Field>
                <Field label="After">
                  <NameList names={data.definition.after} />
                </Field>
              </dl>
            </section>

            <section>
              <h3 className="mb-1 text-xs font-semibold text-text-primary">
                Journal (50 dernières entrées)
              </h3>
              {!data.journal.available && (
                <p className="flex items-center gap-1.5 text-xs text-text-muted">
                  <CircleSlash className="h-3.5 w-3.5" aria-hidden="true" />
                  Journal indisponible.
                </p>
              )}
              {data.journal.available && detail.journal.length === 0 && (
                <p className="flex items-center gap-1.5 text-xs text-text-muted">
                  <CheckCircle2 className="h-3.5 w-3.5" aria-hidden="true" />
                  Aucune entrée pour cette unité.
                </p>
              )}
              {detail.journal.length > 0 && (
                <ol className="max-h-80 space-y-0.5 overflow-y-auto rounded bg-surface p-2 font-mono text-[10px] leading-relaxed">
                  {detail.journal.map((entry) => (
                    <li key={entry.cursor} className="break-words">
                      <span className="text-text-muted">{formatJournalTime(entry.time)}</span>{' '}
                      <span className="text-text-primary">{displayMessage(entry.message)}</span>
                    </li>
                  ))}
                </ol>
              )}
            </section>
          </>
        )}
      </CardBody>
    </Card>
  )
}
