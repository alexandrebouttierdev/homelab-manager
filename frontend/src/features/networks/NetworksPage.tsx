import { Cable, Network } from 'lucide-react'
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
import { StatusDot } from '../../components/ui/StatusDot'
import { containerStateTone } from '../../lib/containers'
import { useNetworksPage } from './useNetworksPage'

export function NetworksPage() {
  const {
    unused,
    search,
    setSearch,
    isLoading,
    error,
    retry,
    attachedContainerCount,
    activeNetworkCount,
    networks,
    filtered,
  } = useNetworksPage()

  return (
    <div className="space-y-4">
      <PageHeader
        title="Réseaux Docker"
        breadcrumb={[{ label: 'Docker' }, { label: 'Réseaux' }]}
        badge={<Badge tone="neutral">{networks.length} réseau(x)</Badge>}
        description="Réseaux auxquels les conteneurs observés sont attachés, puis réseaux créés sans aucun conteneur (bridge, host et none, propres à Docker, exclus). La configuration du démon n'est pas exposée."
      />

      <div className="grid gap-3 sm:grid-cols-3">
        <div className="rounded-lg border border-line bg-card p-4">
          <p className="text-xs text-text-muted">Réseaux distincts</p>
          <p className="mt-1 text-[28px] font-semibold tracking-[-0.03em] text-text-primary">
            {networks.length}
          </p>
        </div>
        <div className="rounded-lg border border-line bg-card p-4">
          <p className="text-xs text-text-muted">Conteneurs attachés</p>
          <p className="mt-1 text-[28px] font-semibold tracking-[-0.03em] text-accent">
            {attachedContainerCount}
          </p>
        </div>
        <div className="rounded-lg border border-line bg-card p-4">
          <p className="text-xs text-text-muted">Réseaux actifs</p>
          <p className="mt-1 text-[28px] font-semibold tracking-[-0.03em] text-success">
            {activeNetworkCount}
          </p>
        </div>
      </div>

      <SearchField
        value={search}
        onChange={setSearch}
        placeholder="Filtrer par réseau ou conteneur…"
        ariaLabel="Rechercher un réseau"
      />

      {isLoading && <LoadingState label="Chargement des réseaux…" rows={3} />}
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
          icon={Cable}
          title={networks.length === 0 ? 'Aucun réseau observé' : 'Aucun réseau ne correspond'}
          description={
            networks.length === 0
              ? 'Les réseaux apparaîtront dès que des conteneurs attachés à un réseau seront découverts.'
              : 'Modifiez la recherche pour retrouver un réseau.'
          }
        />
      )}

      {filtered.length > 0 && (
        <>
          <Table minWidthClassName="min-w-[820px]">
            <THead>
              <tr>
                <TH>Réseau</TH>
                <TH>Conteneurs</TH>
                <TH>Actifs</TH>
                <TH>Membres</TH>
                <TH>États</TH>
              </tr>
            </THead>
            <TBody>
              {filtered.map((network) => (
                <TR key={network.name}>
                  <TD>
                    <span className="flex items-center gap-2">
                      <Network className="h-3.5 w-3.5 text-text-muted" aria-hidden="true" />
                      <span className="font-mono text-xs text-text-primary">{network.name}</span>
                    </span>
                  </TD>
                  <TD>
                    <span className="font-mono text-xs">{network.containers.length}</span>
                  </TD>
                  <TD>
                    <Badge
                      tone={network.running > 0 ? 'success' : 'neutral'}
                      dot={network.running > 0}
                    >
                      {network.running}
                    </Badge>
                  </TD>
                  <TD>
                    <div className="flex max-w-[420px] flex-wrap gap-1">
                      {network.containers.slice(0, 5).map((container) => (
                        <Link
                          key={container.id}
                          to={`/containers/${encodeURIComponent(container.id)}`}
                          className="inline-flex items-center gap-1 rounded-sm border border-line px-1.5 py-0.5 text-xs text-text-secondary transition-colors hover:border-accent/40 hover:text-accent"
                        >
                          <StatusDot
                            tone={containerStateTone(container.state)}
                            className="h-1 w-1"
                          />
                          {container.name}
                        </Link>
                      ))}
                      {network.containers.length > 5 && (
                        <span className="text-xs text-text-muted">
                          +{network.containers.length - 5}
                        </span>
                      )}
                    </div>
                  </TD>
                  <TD>
                    <div className="flex flex-wrap gap-1">
                      {network.states.map((state) => (
                        <Badge key={state} tone="neutral">
                          {state}
                        </Badge>
                      ))}
                    </div>
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
          <CardFooter className="flex items-center justify-between gap-2 rounded-lg border border-line bg-card text-xs text-text-muted">
            <span>{filtered.length} réseau(x) affiché(s)</span>
            <span>Attachements issus de l'inventaire Docker · lecture seule</span>
          </CardFooter>
        </>
      )}
      <UnusedDockerCard
        title="Réseaux inutilisés"
        emptyLabel="Tous les réseaux créés sont reliés à au moins un conteneur."
        section={unused}
      />
    </div>
  )
}
