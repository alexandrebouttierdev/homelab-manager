import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Container as ContainerIcon, ExternalLink, RefreshCw, SearchX } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
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
import {
  getCurrentContainerMetrics,
  listContainers,
  type ContainerInfo,
} from '../../lib/api/monitoring'
import {
  containerStateLabel,
  containerStateTone,
  formatPorts,
  imageTag,
  isErrorState,
  isRunning,
  isStopped,
  shortId,
} from '../../lib/containers'
import { formatBytes, formatPercent, formatUptime } from '../../lib/format'
import { latestOne } from '../../lib/metrics'
import { useRefreshInterval } from '../../lib/refresh'

type StateFilter = 'all' | 'running' | 'stopped' | 'error'
type SortOrder = 'name' | 'state' | 'uptime' | 'restarts'

function ContainerUsageCells({ containerId, enabled }: { containerId: string; enabled: boolean }) {
  const refreshInterval = useRefreshInterval()
  const metrics = useQuery({
    queryKey: ['container', containerId, 'current-metrics'],
    queryFn: () => getCurrentContainerMetrics(containerId),
    enabled,
    staleTime: 15_000,
    refetchInterval: enabled && refreshInterval > 0 ? refreshInterval : false,
  })

  if (!enabled) {
    return (
      <>
        <TD className="font-mono text-[10px] text-text-muted">—</TD>
        <TD className="font-mono text-[10px] text-text-muted">—</TD>
      </>
    )
  }
  if (metrics.isLoading) {
    return (
      <>
        <TD className="font-mono text-[10px] text-text-muted">…</TD>
        <TD className="font-mono text-[10px] text-text-muted">…</TD>
      </>
    )
  }
  if (metrics.isError) {
    return (
      <>
        <TD className="font-mono text-[10px] text-text-muted">indisponible</TD>
        <TD className="font-mono text-[10px] text-text-muted">indisponible</TD>
      </>
    )
  }

  const samples = metrics.data?.samples ?? []
  const cpu = latestOne(samples, 'cpu.percent')?.value ?? null
  const memory = latestOne(samples, 'memory.usage_bytes')?.value ?? null
  const memoryLimit = latestOne(samples, 'memory.limit_bytes')?.value ?? null
  const memoryPercent = latestOne(samples, 'memory.percent')?.value ?? null

  return (
    <>
      <TD>
        <div className="min-w-[78px] space-y-1">
          <span className="block font-mono text-xs text-text-primary">{formatPercent(cpu)}</span>
          <ProgressBar value={cpu} tone="cpu" label="Utilisation CPU du conteneur" />
        </div>
      </TD>
      <TD>
        <div className="min-w-[128px] space-y-1">
          <div className="flex items-baseline justify-between gap-2 font-mono text-[10px]">
            <span className="text-text-primary">{formatPercent(memoryPercent)}</span>
            <span className="text-text-muted">{formatBytes(memory)}</span>
          </div>
          <ProgressBar value={memoryPercent} tone="ram" label="Utilisation mémoire du conteneur" />
          {memoryLimit != null && (
            <span className="block font-mono text-[9px] text-text-muted">
              sur {formatBytes(memoryLimit)}
            </span>
          )}
        </div>
      </TD>
    </>
  )
}

function matchesSearch(container: ContainerInfo, search: string): boolean {
  if (!search) return true
  const haystack = [
    container.name,
    container.image,
    container.dockerId,
    container.status,
    container.state,
    ...container.networks,
    ...formatPorts(container.ports),
  ]
    .join(' ')
    .toLowerCase()
  return haystack.includes(search.toLowerCase())
}

export function ContainersPage() {
  const queryClient = useQueryClient()
  const refreshInterval = useRefreshInterval()
  const [searchParams] = useSearchParams()
  const [search, setSearch] = useState(searchParams.get('image') ?? '')
  const [stateFilter, setStateFilter] = useState<StateFilter>('all')
  const [sortOrder, setSortOrder] = useState<SortOrder>('name')
  const [includeRemoved, setIncludeRemoved] = useState(false)

  const containers = useQuery({
    queryKey: ['containers', includeRemoved],
    queryFn: () => listContainers(includeRemoved),
    refetchInterval: refreshInterval || false,
  })

  const all = useMemo(() => containers.data ?? [], [containers.data])
  const running = all.filter((container) => isRunning(container.state))
  const stopped = all.filter((container) => isStopped(container.state))
  const errors = all.filter((container) => isErrorState(container.state))

  const filtered = useMemo(() => {
    const result = all.filter((container) => {
      if (stateFilter === 'running' && !isRunning(container.state)) return false
      if (stateFilter === 'stopped' && !isStopped(container.state)) return false
      if (stateFilter === 'error' && !isErrorState(container.state)) return false
      return matchesSearch(container, search)
    })

    return [...result].sort((left, right) => {
      switch (sortOrder) {
        case 'state':
          return left.state.localeCompare(right.state) || left.name.localeCompare(right.name)
        case 'uptime':
          return (right.uptimeSeconds ?? -1) - (left.uptimeSeconds ?? -1)
        case 'restarts':
          return right.restartCount - left.restartCount || left.name.localeCompare(right.name)
        default:
          return left.name.localeCompare(right.name)
      }
    })
  }, [all, search, sortOrder, stateFilter])

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
            onClick={() => {
              void queryClient.invalidateQueries({ queryKey: ['containers'] })
            }}
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
            { value: 'all', label: 'Tous', count: all.length },
            { value: 'running', label: 'Actifs', count: running.length },
            { value: 'stopped', label: 'Arrêtés', count: stopped.length },
            { value: 'error', label: 'Incidents', count: errors.length },
          ]}
        />
        <Badge tone={errors.length > 0 ? 'danger' : 'success'} dot={errors.length > 0}>
          {errors.length > 0
            ? `${errors.length} conteneur(s) en incident`
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
          onChange={(event) => setSortOrder(event.target.value as SortOrder)}
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

      {containers.isLoading && <LoadingState label="Chargement des conteneurs…" rows={4} />}
      {containers.isError && (
        <ErrorState
          message={containers.error.message}
          onRetry={() => {
            void containers.refetch()
          }}
        />
      )}

      {!containers.isLoading && !containers.isError && all.length === 0 && (
        <EmptyState
          icon={ContainerIcon}
          title="Aucun conteneur détecté"
          description="Le collecteur Docker ne rapporte actuellement aucun conteneur. Vérifiez que le démon Docker et le proxy local sont actifs."
        />
      )}

      {!containers.isLoading && !containers.isError && all.length > 0 && filtered.length === 0 && (
        <EmptyState
          icon={SearchX}
          title="Aucun conteneur ne correspond"
          description="Modifiez la recherche ou le filtre d'état pour retrouver un conteneur."
          action={
            <Button
              size="sm"
              variant="secondary"
              onClick={() => {
                setSearch('')
                setStateFilter('all')
              }}
            >
              Réinitialiser les filtres
            </Button>
          }
        />
      )}

      {filtered.length > 0 && (
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
              {filtered.map((container, index) => {
                const ports = formatPorts(container.ports)
                const tag = imageTag(container.image)
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
                      <ContainerUsageCells
                        containerId={container.id}
                        enabled={isRunning(container.state) && index < 50}
                      />
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
              {filtered.length} sur {all.length} conteneur(s) affiché(s)
              {all.length > 50 ? ' · charge détaillée limitée aux 50 premiers actifs' : ''}
            </span>
            <span>Découverte automatique via le proxy Docker local · lecture seule</span>
          </CardFooter>
        </>
      )}
    </div>
  )
}
