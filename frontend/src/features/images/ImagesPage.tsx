import { ExternalLink, Images as ImagesIcon } from 'lucide-react'
import { Link } from 'react-router-dom'
import { UnusedDockerCard } from '../../components/UnusedDockerCard'
import { Badge } from '../../components/ui/Badge'
import { CardFooter } from '../../components/ui/Card'
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/DataTable'
import { EmptyState } from '../../components/ui/EmptyState'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { PageHeader } from '../../components/ui/PageHeader'
import { SearchField } from '../../components/ui/SearchField'
import { useImagesPage } from './useImagesPage'

export function ImagesPage() {
  const {
    search,
    setSearch,
    unused,
    isLoading,
    error,
    retry,
    containerCount,
    activeImageCount,
    images,
    filtered,
  } = useImagesPage()

  return (
    <div className="space-y-4">
      <PageHeader
        title="Images Docker"
        breadcrumb={[{ label: 'Docker' }, { label: 'Images' }]}
        badge={<Badge tone="neutral">{images.length} image(s)</Badge>}
        description="Images utilisées par les conteneurs observés, puis images qu'aucun conteneur, même arrêté, n'utilise. Lecture seule : le Manager ne supprime rien."
      />

      <div className="grid gap-3 sm:grid-cols-3">
        <div className="rounded-lg border border-line bg-card p-4">
          <p className="text-xs text-text-muted">Images distinctes</p>
          <p className="mt-1 text-[28px] font-semibold tracking-[-0.03em] text-text-primary">
            {images.length}
          </p>
        </div>
        <div className="rounded-lg border border-line bg-card p-4">
          <p className="text-xs text-text-muted">Conteneurs associés</p>
          <p className="mt-1 text-[28px] font-semibold tracking-[-0.03em] text-text-primary">
            {containerCount}
          </p>
        </div>
        <div className="rounded-lg border border-line bg-card p-4">
          <p className="text-xs text-text-muted">Images actives</p>
          <p className="mt-1 text-[28px] font-semibold tracking-[-0.03em] text-success">
            {activeImageCount}
          </p>
        </div>
      </div>

      <SearchField
        value={search}
        onChange={setSearch}
        placeholder="Filtrer par image, dépôt ou tag…"
        ariaLabel="Rechercher une image"
      />

      {isLoading && <LoadingState label="Chargement des images…" rows={3} />}
      {error && (
        <ErrorState
          message={error.message}
          onRetry={() => {
            retry()
          }}
        />
      )}

      {!isLoading && !error && filtered.length === 0 && (
        <EmptyState
          icon={ImagesIcon}
          title={images.length === 0 ? 'Aucune image observée' : 'Aucune image ne correspond'}
          description={
            images.length === 0
              ? 'Les images apparaîtront dès que des conteneurs seront découverts par le collecteur Docker.'
              : 'Modifiez la recherche pour retrouver une image.'
          }
        />
      )}

      {filtered.length > 0 && (
        <>
          <Table minWidthClassName="min-w-[820px]">
            <THead>
              <tr>
                <TH>Image</TH>
                <TH>Tag</TH>
                <TH>Conteneurs</TH>
                <TH>Actifs</TH>
                <TH>États observés</TH>
                <TH>
                  <span className="sr-only">Action</span>
                </TH>
              </tr>
            </THead>
            <TBody>
              {filtered.map((image) => {
                return (
                  <TR key={image.image}>
                    <TD>
                      <span
                        className="block max-w-[420px] truncate font-mono text-xs text-text-primary"
                        title={image.image}
                      >
                        {image.repository}
                      </span>
                    </TD>
                    <TD>
                      <Badge tone="neutral">{image.tag}</Badge>
                    </TD>
                    <TD>
                      <div className="flex max-w-[280px] flex-wrap gap-1">
                        {image.containers.slice(0, 3).map((container) => (
                          <Link
                            key={container.id}
                            to={`/containers/${encodeURIComponent(container.id)}`}
                            className="rounded-sm border border-line px-1.5 py-0.5 text-xs text-text-secondary transition-colors hover:border-accent/40 hover:text-accent"
                          >
                            {container.name}
                          </Link>
                        ))}
                        {image.containers.length > 3 && (
                          <span className="text-xs text-text-muted">
                            +{image.containers.length - 3}
                          </span>
                        )}
                      </div>
                    </TD>
                    <TD>
                      <Badge
                        tone={image.running > 0 ? 'success' : 'neutral'}
                        dot={image.running > 0}
                      >
                        {image.running} / {image.containers.length}
                      </Badge>
                    </TD>
                    <TD>
                      <div className="flex flex-wrap gap-1">
                        {image.states.map((state) => (
                          <Badge key={state} tone="neutral">
                            {state}
                          </Badge>
                        ))}
                      </div>
                    </TD>
                    <TD>
                      <Link
                        to={`/containers?image=${encodeURIComponent(image.image)}`}
                        className="inline-flex h-7 items-center gap-1.5 rounded-md border border-line px-2 text-xs text-text-secondary transition-colors hover:bg-elevated hover:text-text-primary"
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
          <CardFooter className="flex items-center justify-between gap-2 rounded-lg border border-line bg-card text-xs text-text-muted">
            <span>{filtered.length} image(s) affichée(s)</span>
            <span>Inventaire dérivé · lecture seule</span>
          </CardFooter>
        </>
      )}
      <UnusedDockerCard
        title="Images inutilisées"
        emptyLabel="Toutes les images servent à au moins un conteneur."
        section={unused}
      />
    </div>
  )
}
