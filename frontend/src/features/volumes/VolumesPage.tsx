import { Database, HardDrive } from 'lucide-react'
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
import { useVolumesPage } from './useVolumesPage'

export function VolumesPage() {
  const { search, setSearch, unused, containers, mounts, filtered, bindMounts, namedVolumes } =
    useVolumesPage()

  return (
    <div className="space-y-4">
      <PageHeader
        title="Volumes & montages"
        breadcrumb={[{ label: 'Docker' }, { label: 'Volumes' }]}
        badge={<Badge tone="neutral">{mounts.length} montage(s)</Badge>}
        description="Montages déclarés par les conteneurs observés, puis volumes nommés qu'aucun conteneur, même arrêté, ne monte. Le Manager n'expose ni création, ni suppression des volumes Docker."
      />

      <div className="grid gap-3 sm:grid-cols-3">
        <div className="rounded-lg border border-line bg-card p-4">
          <p className="text-xs text-text-muted">Montages distincts</p>
          <p className="mt-1 text-[28px] font-semibold tracking-[-0.03em] text-text-primary">
            {mounts.length}
          </p>
        </div>
        <div className="rounded-lg border border-line bg-card p-4">
          <p className="text-xs text-text-muted">Volumes nommés</p>
          <p className="mt-1 text-[28px] font-semibold tracking-[-0.03em] text-accent">
            {namedVolumes}
          </p>
        </div>
        <div className="rounded-lg border border-line bg-card p-4">
          <p className="text-xs text-text-muted">Montages bind</p>
          <p className="mt-1 text-[28px] font-semibold tracking-[-0.03em] text-success">
            {bindMounts}
          </p>
        </div>
      </div>

      <SearchField
        value={search}
        onChange={setSearch}
        placeholder="Filtrer par source, destination, type ou conteneur…"
        ariaLabel="Rechercher un montage"
      />

      {containers.isLoading && <LoadingState label="Chargement des montages…" rows={3} />}
      {containers.isError && (
        <ErrorState
          message={containers.error.message}
          onRetry={() => {
            void containers.refetch()
          }}
        />
      )}

      {!containers.isLoading && !containers.isError && filtered.length === 0 && (
        <EmptyState
          icon={Database}
          title={mounts.length === 0 ? 'Aucun montage observé' : 'Aucun montage ne correspond'}
          description={
            mounts.length === 0
              ? 'Les volumes et montages apparaîtront dès que des conteneurs avec des montages seront découverts.'
              : 'Modifiez la recherche pour retrouver un montage.'
          }
        />
      )}

      {filtered.length > 0 && (
        <>
          <Table minWidthClassName="min-w-[900px]">
            <THead>
              <tr>
                <TH>Source</TH>
                <TH>Destination</TH>
                <TH>Type</TH>
                <TH>Accès</TH>
                <TH>Conteneurs</TH>
              </tr>
            </THead>
            <TBody>
              {filtered.map((mount) => (
                <TR key={mount.key}>
                  <TD>
                    <span className="flex items-start gap-2">
                      <HardDrive
                        className="mt-0.5 h-3.5 w-3.5 shrink-0 text-text-muted"
                        aria-hidden="true"
                      />
                      <span
                        className="max-w-[320px] truncate font-mono text-xs text-text-primary"
                        title={mount.source}
                      >
                        {mount.source || '—'}
                      </span>
                    </span>
                  </TD>
                  <TD>
                    <span className="font-mono text-xs">{mount.destination || '—'}</span>
                  </TD>
                  <TD>
                    <Badge tone="neutral">{mount.type || '—'}</Badge>
                  </TD>
                  <TD>
                    <span className="text-xs">{mount.access || '—'}</span>
                  </TD>
                  <TD>
                    <div className="flex max-w-[320px] flex-wrap gap-1">
                      {mount.containers.slice(0, 4).map((container) => (
                        <Link
                          key={container.id}
                          to={`/containers/${encodeURIComponent(container.id)}`}
                          className="rounded-sm border border-line px-1.5 py-0.5 text-xs text-text-secondary transition-colors hover:border-accent/40 hover:text-accent"
                        >
                          {container.name}
                        </Link>
                      ))}
                      {mount.containers.length > 4 && (
                        <span className="text-xs text-text-muted">
                          +{mount.containers.length - 4}
                        </span>
                      )}
                    </div>
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
          <CardFooter className="flex items-center justify-between gap-2 rounded-lg border border-line bg-card text-xs text-text-muted">
            <span>{filtered.length} montage(s) affiché(s)</span>
            <span>Données issues de l'inventaire Docker · lecture seule</span>
          </CardFooter>
        </>
      )}
      <UnusedDockerCard
        title="Volumes nommés inutilisés"
        emptyLabel="Tous les volumes nommés sont montés par au moins un conteneur."
        section={unused}
      />
    </div>
  )
}
