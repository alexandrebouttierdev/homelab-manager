import { Container as ContainerIcon, ExternalLink, RefreshCw, SearchX } from 'lucide-react'
import { Link } from 'react-router-dom'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { CardFooter } from '../../components/ui/Card'
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/DataTable'
import { EmptyState } from '../../components/ui/EmptyState'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { PageHeader } from '../../components/ui/PageHeader'
import { ProgressBar } from '../../components/ui/ProgressBar'
import { SearchField } from '../../components/ui/SearchField'
import { SegmentedTabs } from '../../components/ui/SegmentedTabs'
import { Select } from '../../components/ui/Select'
import { StatusDot } from '../../components/ui/StatusDot'
import { containerStateLabel, containerStateTone, isRunning, shortId } from '../../lib/containers'
import { formatBytes, formatPercent, formatUptime } from '../../lib/format'
import type { ContainerSortOrder } from '../../lib/containers'
import { useContainerUsage } from './useContainerUsage'
import { maxUsageRows, useContainersPage } from './useContainersPage'

/** CPU and memory of one container, rendered inside the "Charge" cell. */
function ContainerUsage({ containerId, enabled }: { containerId: string; enabled: boolean }) {
  const usage = useContainerUsage(containerId, enabled)
  if (usage.state !== 'ok') {
    const label = { disabled: '—', loading: '…', error: 'indisponible' }[usage.state]
    return <span className="font-mono text-[10px] text-text-muted">{label}</span>
  }
  return (
    <div className="min-w-[150px] space-y-1.5">
      <div className="space-y-1">
        <div className="flex items-baseline justify-between gap-2 font-mono text-[10px]">
          <span className="text-text-muted">CPU</span>
          <span className="text-text-primary">{formatPercent(usage.cpu)}</span>
        </div>
        <ProgressBar value={usage.cpu} tone="cpu" label="Utilisation CPU du conteneur" />
      </div>
      <div className="space-y-1">
        <div className="flex items-baseline justify-between gap-2 font-mono text-[10px]">
          <span className="text-text-muted">RAM {formatPercent(usage.memoryPercent)}</span>
          <span className="text-text-primary">
            {formatBytes(usage.memory)}
            {usage.memoryLimit != null && (
              <span className="text-text-muted"> / {formatBytes(usage.memoryLimit)}</span>
            )}
          </span>
        </div>
        <ProgressBar
          value={usage.memoryPercent}
          tone="ram"
          label="Utilisation mémoire du conteneur"
        />
      </div>
    </div>
  )
}

export function ContainersPage() {
  const {
    search,
    setSearch,
    stateFilter,
    setStateFilter,
    sortOrder,
    setSortOrder,
    includeRemoved,
    setIncludeRemoved,
    counts,
    rows,
    isLoading,
    error,
    retry,
    refresh,
    resetFilters,
  } = useContainersPage()

  return (
    <div className="space-y-4">
      <PageHeader
        title="Conteneurs Docker"
        breadcrumb={[{ label: 'Docker' }, { label: 'Conteneurs' }]}
        badge={<Badge tone="neutral">Docker local</Badge>}
        description="Inventaire en lecture seule des conteneurs découverts sur la machine. Aucune action de démarrage, d'arrêt ou d'administration n'est exposée."
        actions={
          <Button
            size="sm"
            variant="secondary"
            icon={<RefreshCw className="h-3.5 w-3.5" />}
            onClick={refresh}
          >
            Rafraîchir
          </Button>
        }
      />

      <div className="flex flex-wrap items-center justify-between gap-2">
        <SegmentedTabs
          ariaLabel="Filtrer les conteneurs par état"
          value={stateFilter}
          onChange={setStateFilter}
          options={[
            { value: 'all', label: 'Tous', count: counts.all },
            { value: 'running', label: 'Actifs', count: counts.running },
            { value: 'stopped', label: 'Arrêtés', count: counts.stopped },
            { value: 'error', label: 'Incidents', count: counts.errors },
          ]}
        />
        <Badge tone={counts.errors > 0 ? 'danger' : 'success'} dot={counts.errors > 0}>
          {counts.errors > 0
            ? `${counts.errors} conteneur(s) en incident`
            : 'Aucun incident détecté'}
        </Badge>
      </div>

      <div className="flex flex-wrap items-center gap-2 rounded-lg border border-line bg-card p-2.5">
        <SearchField
          value={search}
          onChange={setSearch}
          placeholder="Filtrer par nom, image, port, réseau…"
          ariaLabel="Rechercher un conteneur"
          className="min-w-[220px] flex-1 border-0 bg-surface"
        />
        <Select
          aria-label="Trier les conteneurs"
          value={sortOrder}
          onChange={(event) => setSortOrder(event.target.value as ContainerSortOrder)}
          className="min-w-[150px]"
        >
          <option value="name">Trier par nom</option>
          <option value="state">Trier par état</option>
          <option value="uptime">Trier par uptime</option>
          <option value="restarts">Trier par redémarrages</option>
        </Select>
        <label className="flex cursor-pointer select-none items-center gap-2 px-2 text-xs text-text-secondary">
          <input
            type="checkbox"
            checked={includeRemoved}
            onChange={(event) => setIncludeRemoved(event.target.checked)}
            className="h-3.5 w-3.5 rounded-sm border-line bg-surface accent-accent"
          />
          Inclure les supprimés
        </label>
      </div>

      {isLoading && <LoadingState label="Chargement des conteneurs…" rows={4} />}
      {error && (
        <ErrorState
          message={error.message}
          onRetry={() => {
            retry()
          }}
        />
      )}

      {!isLoading && !error && counts.all === 0 && (
        <EmptyState
          icon={ContainerIcon}
          title="Aucun conteneur détecté"
          description="Le collecteur Docker ne rapporte actuellement aucun conteneur. Vérifiez que le démon Docker et le proxy local sont actifs."
        />
      )}

      {!isLoading && !error && counts.all > 0 && rows.length === 0 && (
        <EmptyState
          icon={SearchX}
          title="Aucun conteneur ne correspond"
          description="Modifiez la recherche ou le filtre d'état pour retrouver un conteneur."
          action={
            <Button size="sm" variant="secondary" onClick={resetFilters}>
              Réinitialiser les filtres
            </Button>
          }
        />
      )}

      {rows.length > 0 && (
        <>
          <Table minWidthClassName="min-w-[1180px]">
            <THead>
              <tr>
                <TH>Conteneur</TH>
                <TH>Image</TH>
                <TH>État &amp; santé</TH>
                <TH>Uptime</TH>
                <TH>Redémarrages</TH>
                <TH>Ports</TH>
                <TH>Réseaux</TH>
                <TH>Charge</TH>
                <TH>
                  <span className="sr-only">Détail</span>
                </TH>
              </tr>
            </THead>
            <TBody>
              {rows.map(({ container, ports, tag, showUsage }) => {
                return (
                  <TR key={container.id}>
                    <TD>
                      <div className="flex items-start gap-2.5">
                        <StatusDot
                          tone={containerStateTone(container.state)}
                          pulse={isRunning(container.state)}
                          label={containerStateLabel(container.state)}
                          className="mt-1.5"
                        />
                        <div className="min-w-0">
                          <Link
                            to={`/containers/${encodeURIComponent(container.id)}`}
                            className="block truncate text-sm font-medium text-text-primary transition-colors hover:text-accent"
                          >
                            {container.name || 'Sans nom'}
                          </Link>
                          <span className="mt-0.5 block font-mono text-[10px] text-text-muted">
                            {shortId(container.dockerId)}
                            {container.removedAt && ' · supprimé'}
                          </span>
                        </div>
                      </div>
                    </TD>
                    <TD>
                      <span
                        className="block max-w-[240px] truncate font-mono text-xs text-text-secondary"
                        title={container.image}
                      >
                        {tag.repository}
                      </span>
                      <Badge tone="neutral" className="mt-1">
                        {tag.tag}
                      </Badge>
                    </TD>
                    <TD>
                      <Badge tone={containerStateTone(container.state)} dot>
                        {containerStateLabel(container.state)}
                      </Badge>
                      <p
                        className="mt-1 max-w-[220px] truncate font-mono text-[10px] text-text-muted"
                        title={container.status}
                      >
                        {container.status || '—'}
                      </p>
                    </TD>
                    <TD className="font-mono text-xs">
                      {isRunning(container.state) ? formatUptime(container.uptimeSeconds) : '—'}
                    </TD>
                    <TD>
                      <Badge tone={container.restartCount > 0 ? 'warning' : 'neutral'}>
                        {container.restartCount}
                      </Badge>
                    </TD>
                    <TD>
                      {ports.length === 0 ? (
                        <span className="font-mono text-[10px] text-text-muted">
                          Aucun port publié
                        </span>
                      ) : (
                        <div className="space-y-1 font-mono text-[10px] text-text-secondary">
                          {ports.slice(0, 2).map((port) => (
                            <span key={port} className="block whitespace-nowrap">
                              {port}
                            </span>
                          ))}
                          {ports.length > 2 && (
                            <span className="text-text-muted">+{ports.length - 2} autre(s)</span>
                          )}
                        </div>
                      )}
                    </TD>
                    <TD>
                      {container.networks.length === 0 ? (
                        <span className="font-mono text-[10px] text-text-muted">—</span>
                      ) : (
                        <div className="flex max-w-[180px] flex-wrap gap-1">
                          {container.networks.slice(0, 3).map((network) => (
                            <Badge key={network} tone="neutral">
                              {network}
                            </Badge>
                          ))}
                          {container.networks.length > 3 && (
                            <Badge tone="neutral">+{container.networks.length - 3}</Badge>
                          )}
                        </div>
                      )}
                    </TD>
                    <TD>
                      <ContainerUsage containerId={container.id} enabled={showUsage} />
                    </TD>
                    <TD>
                      <Link
                        to={`/containers/${encodeURIComponent(container.id)}`}
                        className="inline-flex h-7 w-7 items-center justify-center rounded-md border border-line text-text-muted transition-colors hover:bg-elevated hover:text-text-primary"
                        aria-label={`Voir le détail de ${container.name}`}
                      >
                        <ExternalLink className="h-3.5 w-3.5" />
                      </Link>
                    </TD>
                  </TR>
                )
              })}
            </TBody>
          </Table>

          <CardFooter className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-line bg-card font-mono text-[10px] text-text-muted">
            <span>
              {rows.length} sur {counts.all} conteneur(s) affiché(s)
              {counts.all > maxUsageRows
                ? ` · charge détaillée limitée aux ${maxUsageRows} premiers actifs`
                : ''}
            </span>
            <span>Découverte automatique via le proxy Docker local · lecture seule</span>
          </CardFooter>
        </>
      )}
    </div>
  )
}
