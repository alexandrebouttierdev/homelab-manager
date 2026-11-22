import { useQuery } from '@tanstack/react-query'
import { ExternalLink, Images as ImagesIcon } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { Badge } from '../../components/ui/Badge'
import { CardFooter } from '../../components/ui/Card'
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/DataTable'
import { EmptyState } from '../../components/ui/EmptyState'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { PageHeader } from '../../components/ui/PageHeader'
import { SearchField } from '../../components/ui/SearchField'
import { listContainers } from '../../lib/api/monitoring'
import { imageTag, isRunning } from '../../lib/containers'
import { useRefreshInterval } from '../../lib/refresh'

type ImageGroup = {
  image: string
  repository: string
  tag: string
  containers: Array<{ id: string; name: string; state: string }>
  running: number
}

export function ImagesPage() {
  const refreshInterval = useRefreshInterval()
  const [search, setSearch] = useState('')
  const containers = useQuery({
    queryKey: ['containers', false],
    queryFn: () => listContainers(false),
    refetchInterval: refreshInterval || false,
  })

  const images = useMemo(() => {
    const grouped = new Map<string, ImageGroup>()
    for (const container of containers.data ?? []) {
      const existing = grouped.get(container.image) ?? {
        image: container.image,
        ...imageTag(container.image),
        containers: [],
        running: 0,
      }
      existing.containers.push({ id: container.id, name: container.name, state: container.state })
      if (isRunning(container.state)) existing.running += 1
      grouped.set(container.image, existing)
    }
    return Array.from(grouped.values()).sort((left, right) => left.image.localeCompare(right.image))
  }, [containers.data])

  const filtered = images.filter((image) =>
    !search || `${image.image} ${image.repository} ${image.tag}`.toLowerCase().includes(search.toLowerCase()),
  )

  return (
    <div className="space-y-4">
      <PageHeader
        title="Images Docker"
        breadcrumb={[{ label: 'Docker' }, { label: 'Images' }]}
        badge={<Badge tone="neutral">{images.length} image(s)</Badge>}
        description="Images dérivées de l'inventaire des conteneurs observés. Les images non utilisées ne sont pas visibles via l'API Docker en lecture seule du Manager."
      />

      <div className="grid gap-3 sm:grid-cols-3">
        <div className="rounded-lg border border-line bg-card p-4">
          <p className="font-mono text-[11px] uppercase tracking-wider text-text-muted">Images distinctes</p>
          <p className="mt-1 font-mono text-2xl font-semibold text-text-primary">{images.length}</p>
        </div>
        <div className="rounded-lg border border-line bg-card p-4">
          <p className="font-mono text-[11px] uppercase tracking-wider text-text-muted">Conteneurs associés</p>
          <p className="mt-1 font-mono text-2xl font-semibold text-text-primary">
            {(containers.data ?? []).length}
          </p>
        </div>
        <div className="rounded-lg border border-line bg-card p-4">
          <p className="font-mono text-[11px] uppercase tracking-wider text-text-muted">Images actives</p>
          <p className="mt-1 font-mono text-2xl font-semibold text-success">
            {images.filter((image) => image.running > 0).length}
          </p>
        </div>
      </div>

      <SearchField
        value={search}
        onChange={setSearch}
        placeholder="Filtrer par image, dépôt ou tag…"
        ariaLabel="Rechercher une image"
      />

      {containers.isLoading && <LoadingState label="Chargement des images…" rows={3} />}
      {containers.isError && <ErrorState message={containers.error.message} onRetry={() => { void containers.refetch() }} />}

      {!containers.isLoading && !containers.isError && filtered.length === 0 && (
        <EmptyState
          icon={ImagesIcon}
          title={images.length === 0 ? 'Aucune image observée' : 'Aucune image ne correspond'}
          description={images.length === 0
            ? 'Les images apparaîtront dès que des conteneurs seront découverts par le collecteur Docker.'
            : 'Modifiez la recherche pour retrouver une image.'}
        />
      )}

      {filtered.length > 0 && (
        <>
          <Table minWidthClassName="min-w-[820px]">
            <THead>
              <tr><TH>Image</TH><TH>Tag</TH><TH>Conteneurs</TH><TH>Actifs</TH><TH>États observés</TH><TH><span className="sr-only">Action</span></TH></tr>
            </THead>
            <TBody>
              {filtered.map((image) => {
                const states = Array.from(new Set(image.containers.map((container) => container.state)))
                return (
                  <TR key={image.image}>
                    <TD>
                      <span className="block max-w-[420px] truncate font-mono text-xs text-text-primary" title={image.image}>
                        {image.repository}
                      </span>
                    </TD>
                    <TD><Badge tone="neutral">{image.tag}</Badge></TD>
                    <TD>
                      <div className="flex max-w-[280px] flex-wrap gap-1">
                        {image.containers.slice(0, 3).map((container) => (
                          <Link
                            key={container.id}
                            to={`/containers/${encodeURIComponent(container.id)}`}
                            className="rounded-sm border border-line px-1.5 py-0.5 font-mono text-[10px] text-text-secondary transition-colors hover:border-accent/40 hover:text-accent"
                          >
                            {container.name}
                          </Link>
                        ))}
                        {image.containers.length > 3 && (
                          <span className="font-mono text-[10px] text-text-muted">+{image.containers.length - 3}</span>
                        )}
                      </div>
                    </TD>
                    <TD>
                      <Badge tone={image.running > 0 ? 'success' : 'neutral'} dot={image.running > 0}>
                        {image.running} / {image.containers.length}
                      </Badge>
                    </TD>
                    <TD>
                      <div className="flex flex-wrap gap-1">
                        {states.map((state) => <Badge key={state} tone="neutral">{state}</Badge>)}
                      </div>
                    </TD>
                    <TD>
                      <Link
                        to={`/containers?image=${encodeURIComponent(image.image)}`}
                        className="inline-flex h-7 items-center gap-1.5 rounded-md border border-line px-2 font-mono text-[10px] text-text-secondary transition-colors hover:bg-elevated hover:text-text-primary"
                      >
                        <ExternalLink className="h-3 w-3" />
                        Conteneurs
                      </Link>
                    </TD>
                  </TR>
                )
              })}
            </TBody>
          </Table>
          <CardFooter className="flex items-center justify-between gap-2 rounded-lg border border-line bg-card font-mono text-[10px] text-text-muted">
            <span>{filtered.length} image(s) affichée(s)</span>
            <span>Inventaire dérivé · lecture seule</span>
          </CardFooter>
        </>
      )}
    </div>
  )
}
