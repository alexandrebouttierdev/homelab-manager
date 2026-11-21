import { useQuery } from '@tanstack/react-query'
import { getHost, getHostTemperatures } from '../../lib/api/monitoring'

export function HostPage() {
  const host = useQuery({ queryKey: ['host'], queryFn: getHost, refetchInterval: 30_000 })
  const temperatures = useQuery({
    queryKey: ['host', 'temperatures'],
    queryFn: getHostTemperatures,
    refetchInterval: 30_000,
  })

  return (
    <section className="content">
      <header>
        <div><p className="eyebrow">Machine locale</p><h1>Hôte</h1></div>
        <span className="status-pill">{host.data?.collector.status ?? 'indisponible'}</span>
      </header>
      {host.isLoading && <p>Chargement des informations système...</p>}
      {host.isError && <p className="field-error" role="alert">{host.error.message}</p>}
      {host.data && (
        <div className="metrics-grid">
          <article><span>Nom d'hôte</span><strong>{host.data.hostname || 'Indisponible'}</strong></article>
          <article><span>Distribution</span><strong>{host.data.os.distribution || 'Indisponible'}</strong><small>{host.data.os.version}</small></article>
          <article><span>Noyau</span><strong>{host.data.kernelVersion || 'Indisponible'}</strong></article>
          <article><span>Uptime</span><strong>{host.data.uptimeSeconds == null ? 'Indisponible' : `${Math.floor(host.data.uptimeSeconds / 3600)} h`}</strong></article>
          <article><span>Démarrage</span><strong>{host.data.bootTime ? new Date(host.data.bootTime).toLocaleString('fr-FR') : 'Indisponible'}</strong></article>
          <article><span>Dernière collecte</span><strong>{host.data.collector.lastSuccessAt ? new Date(host.data.collector.lastSuccessAt).toLocaleTimeString('fr-FR') : 'Aucune'}</strong></article>
        </div>
      )}
      <article className="chart-card">
        <h2>Températures disponibles</h2>
        {temperatures.isLoading && <p>Recherche des capteurs...</p>}
        {temperatures.isError && <p className="field-error" role="alert">{temperatures.error.message}</p>}
        {!temperatures.isLoading && !temperatures.isError && temperatures.data?.length === 0 && (
          <p>Aucun capteur de température n'est exposé par le matériel ou le système.</p>
        )}
        {temperatures.data?.map((sample) => (
          <p key={`${sample.name}-${sample.recordedAt}-${sample.labels?.sensor ?? ''}`}>
            {sample.labels?.sensor ?? 'Capteur'} : {sample.value.toFixed(1)} °C
          </p>
        ))}
      </article>
    </section>
  )
}
