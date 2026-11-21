import { lazy, Suspense, useState } from 'react'
import type { EChartsOption } from 'echarts'
import { useQuery } from '@tanstack/react-query'
import { getHostMetrics } from '../../lib/api/monitoring'

const ECharts = lazy(() => import('./MetricsChart'))
const metricChoices = [
  { name: 'cpu.percent', label: 'Utilisation CPU' },
  { name: 'memory.used', label: 'Mémoire utilisée' },
  { name: 'swap.used', label: 'Swap utilisée' },
]

export function MetricsPage() {
  const [metricName, setMetricName] = useState(metricChoices[0].name)
  const metrics = useQuery({
    queryKey: ['host-metrics', metricName],
    queryFn: () => getHostMetrics(metricName, new Date(Date.now() - 24 * 60 * 60 * 1000)),
    refetchInterval: 30_000,
  })
  const samples = metrics.data?.filter((sample) =>
    metricName !== 'cpu.percent' || sample.labels?.core === 'all',
  ) ?? []
  const selectedMetric = metricChoices.find((metric) => metric.name === metricName)
  const option: EChartsOption = {
    tooltip: { trigger: 'axis' },
    grid: { left: 56, right: 24, top: 24, bottom: 40 },
    xAxis: { type: 'category', data: samples.map((sample) => new Date(sample.recordedAt).toLocaleTimeString('fr-FR')) },
    yAxis: { type: 'value', name: samples[0]?.unit ?? '' },
    series: [{ type: 'line', name: selectedMetric?.label, smooth: true, data: samples.map((sample) => sample.value) }],
  }

  return (
    <section className="content">
      <header><div><p className="eyebrow">Historique hôte</p><h1>Métriques</h1></div></header>
      <label htmlFor="metric-select">Métrique</label>
      <select id="metric-select" value={metricName} onChange={(event) => setMetricName(event.target.value)}>
        {metricChoices.map((metric) => <option key={metric.name} value={metric.name}>{metric.label}</option>)}
      </select>
      {metrics.isLoading && <p>Chargement des métriques...</p>}
      {metrics.isError && <p className="field-error" role="alert">{metrics.error.message}</p>}
      {!metrics.isLoading && !metrics.isError && samples.length === 0 && (
        <article className="chart-card"><h2>Aucun échantillon</h2><p>Cette métrique n'a pas encore été collectée ou n'est pas disponible sur cet hôte.</p></article>
      )}
      {samples.length > 0 && (
        <article className="chart-card">
          <h2>{selectedMetric?.label} — 24 dernières heures</h2>
          <Suspense fallback={<p>Chargement du graphique...</p>}><ECharts option={option} /></Suspense>
        </article>
      )}
    </section>
  )
}
