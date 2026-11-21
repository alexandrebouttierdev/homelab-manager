import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { useState } from 'react'
import { listContainers } from '../../lib/api/monitoring'

export function ContainersPage() {
  const [includeRemoved, setIncludeRemoved] = useState(false)
  const containers = useQuery({
    queryKey: ['containers', includeRemoved],
    queryFn: () => listContainers(includeRemoved),
    refetchInterval: 30_000,
  })

  return (
    <section className="content">
      <header><div><p className="eyebrow">Docker local</p><h1>Conteneurs</h1></div></header>
      <label className="checkbox-label">
        <input type="checkbox" checked={includeRemoved} onChange={(event) => setIncludeRemoved(event.target.checked)} />
        Inclure les conteneurs supprimés
      </label>
      {containers.isLoading && <p>Chargement des conteneurs...</p>}
      {containers.isError && <p className="field-error" role="alert">{containers.error.message}</p>}
      {!containers.isLoading && !containers.isError && containers.data?.length === 0 && (
        <article className="chart-card"><h2>Aucun conteneur détecté</h2><p>Le collecteur Docker ne rapporte actuellement aucun conteneur.</p></article>
      )}
      {containers.data && containers.data.length > 0 && (
        <div className="table-scroll">
          <table>
            <thead><tr><th>Nom</th><th>Image</th><th>État</th><th>Statut</th><th>Redémarrages</th><th>Dernière observation</th></tr></thead>
            <tbody>
              {containers.data.map((container) => (
                <tr key={container.id}>
                  <td><Link to={`/containers/${encodeURIComponent(container.id)}`}>{container.name}</Link></td><td>{container.image}</td><td>{container.state}</td>
                  <td>{container.status}</td><td>{container.restartCount}</td>
                  <td>{new Date(container.lastObservedAt).toLocaleString('fr-FR')}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}
