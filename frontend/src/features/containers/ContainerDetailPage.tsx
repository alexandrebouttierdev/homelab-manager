import { lazy, Suspense } from 'react'
import type { EChartsOption } from 'echarts'
import { useQuery } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { getContainer, getContainerMetrics } from '../../lib/api/monitoring'

const ECharts = lazy(() => import('../metrics/MetricsChart'))

export function ContainerDetailPage() {
  const containerID = useParams().containerID ?? ''
  const container = useQuery({
    queryKey: ['container', containerID],
    queryFn: () => getContainer(containerID),
    enabled: Boolean(containerID),
    refetchInterval: 30_000,
  })
  const cpu = useQuery({
    queryKey: ['container-metrics', containerID, 'cpu.percent'],
    queryFn: () => getContainerMetrics(containerID, 'cpu.percent', new Date(Date.now() - 24 * 60 * 60 * 1000)),
    enabled: Boolean(containerID),
    refetchInterval: 30_000,
  })
  const samples = cpu.data ?? []
  const option: EChartsOption = {
    tooltip: { trigger: 'axis' },
    grid: { left: 56, right: 24, top: 24, bottom: 40 },
    xAxis: { type: 'category', data: samples.map((sample) => new Date(sample.recordedAt).toLocaleTimeString('fr-FR')) },
    yAxis: { type: 'value', name: '%', min: 0 },
    series: [{ type: 'line', name: 'CPU', smooth: true, data: samples.map((sample) => sample.value) }],
  }

  return (
    <section className="content">
      <header><div><p className="eyebrow"><Link to="/containers">Conteneurs</Link></p><h1>{container.data?.name ?? 'Détail du conteneur'}</h1></div></header>
      {container.isLoading && <p>Chargement du conteneur...</p>}
      {container.isError && <p className="field-error" role="alert">{container.error.message}</p>}
      {container.data && (
        <div className="metrics-grid">
          <article><span>Image</span><strong>{container.data.image}</strong></article>
          <article><span>État</span><strong>{container.data.state}</strong><small>{container.data.status}</small></article>
          <article><span>Redémarrages</span><strong>{container.data.restartCount}</strong><small>{container.data.restartPolicy || 'Politique inconnue'}</small></article>
          <article><span>Uptime</span><strong>{container.data.uptimeSeconds == null ? 'Indisponible' : `${Math.floor(container.data.uptimeSeconds / 3600)} h`}</strong></article>
          <article><span>Réseaux</span><strong>{container.data.networks.join(', ') || 'Aucun'}</strong></article>
          <article><span>Ports</span><strong>{container.data.ports.length || 'Aucun'}</strong></article>
        </div>
      )}
      <article className="chart-card">
        <h2>Historique CPU brut</h2>
        {cpu.isError && <p className="field-error" role="alert">{cpu.error.message}</p>}
        {!cpu.isLoading && !cpu.isError && samples.length === 0 && <p>Aucun échantillon CPU disponible pour ce conteneur.</p>}
        {samples.length > 0 && <Suspense fallback={<p>Chargement du graphique...</p>}><ECharts option={option} /></Suspense>}
      </article>
      {container.data && (
        <article className="chart-card">
          <h2>Volumes montés</h2>
          {container.data.mounts.length === 0 ? <p>Aucun montage rapporté.</p> : (
            <div className="table-scroll">
              <table><thead><tr><th>Source</th><th>Destination</th><th>Type</th><th>Lecture seule</th></tr></thead>
                <tbody>{container.data.mounts.map((mount, index) => (
                  <tr key={`${String(mount.source)}-${index}`}><td>{String(mount.source ?? '')}</td><td>{String(mount.destination ?? '')}</td><td>{String(mount.type ?? '')}</td><td>{String(mount.rw) === 'false' ? 'Oui' : 'Non'}</td></tr>
                ))}</tbody>
              </table>
            </div>
          )}
        </article>
      )}
    </section>
  )
}
