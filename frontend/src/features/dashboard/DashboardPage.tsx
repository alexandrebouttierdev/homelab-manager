import { useQuery } from '@tanstack/react-query'
import { getDashboard, type DashboardData } from '../../lib/api/monitoring'

function latestMetric(metrics: DashboardData['metrics'], name: string) {
  return metrics.filter((sample) => sample.name === name).sort(
    (left, right) => right.recordedAt.localeCompare(left.recordedAt),
  )[0]
}

export function DashboardPage() {
  const dashboard = useQuery({ queryKey: ['dashboard'], queryFn: getDashboard, refetchInterval: 15_000 })
  const cpu = dashboard.data ? latestMetric(dashboard.data.metrics, 'cpu.percent') : undefined
  const memory = dashboard.data ? latestMetric(dashboard.data.metrics, 'memory.used') : undefined
  const memoryTotal = dashboard.data ? latestMetric(dashboard.data.metrics, 'memory.total') : undefined
  const healthy = dashboard.data?.collectors.system.status === 'ok'
    && dashboard.data.collectors.docker.status === 'ok'

  return (
    <section className="content">
      <header>
        <div><p className="eyebrow">Supervision locale</p><h1>Vue d'ensemble</h1></div>
        <span className="status-pill">{healthy ? 'Collectes opérationnelles' : 'Collecte dégradée'}</span>
      </header>
      {dashboard.isLoading && <p>Chargement de la supervision...</p>}
      {dashboard.isError && <p className="field-error" role="alert">{dashboard.error.message}</p>}
      {dashboard.data && (
        <>
          <div className="metrics-grid">
            <article><span>Machine</span><strong>{dashboard.data.host.hostname || 'Indisponible'}</strong><small>{dashboard.data.host.collector.status}</small></article>
            <article><span>CPU</span><strong>{cpu ? `${cpu.value.toFixed(1)} %` : '—'}</strong><small>{cpu ? 'Dernière mesure disponible' : 'Donnée indisponible'}</small></article>
            <article><span>Mémoire</span><strong>{memory ? `${(memory.value / 1024 ** 3).toFixed(1)} Go` : '—'}</strong><small>{memory && memoryTotal ? `sur ${(memoryTotal.value / 1024 ** 3).toFixed(1)} Go` : 'Donnée indisponible'}</small></article>
            <article><span>Conteneurs actifs</span><strong>{dashboard.data.containers.running}</strong><small>{dashboard.data.containers.total} détecté(s), {dashboard.data.containers.stopped} arrêté(s)</small></article>
          </div>
          <article className="chart-card">
            <h2>État des collecteurs</h2>
            <div className="collector-list">
              {Object.entries(dashboard.data.collectors).map(([name, collector]) => (
                <p key={name}>
                  <strong>{name === 'system' ? 'Système' : 'Docker'} :</strong> {collector.status}
                  {collector.lastSuccessAt && ` — dernière réussite ${new Date(collector.lastSuccessAt).toLocaleTimeString('fr-FR')}`}
                </p>
              ))}
            </div>
          </article>
        </>
      )}
    </section>
  )
}
