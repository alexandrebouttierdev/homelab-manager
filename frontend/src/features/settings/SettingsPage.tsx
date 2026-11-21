import { useQuery } from '@tanstack/react-query'
import { getSettings } from '../../lib/api/monitoring'

export function SettingsPage() {
  const settings = useQuery({ queryKey: ['settings'], queryFn: getSettings })
  return (
    <section className="content">
      <header><div><p className="eyebrow">Configuration effective</p><h1>Paramètres</h1></div></header>
      {settings.isLoading && <p>Chargement des paramètres...</p>}
      {settings.isError && <p className="field-error" role="alert">{settings.error.message}</p>}
      {settings.data && (
        <>
          <div className="metrics-grid">
            <article><span>Collecte système</span><strong>{settings.data.systemCollectIntervalSeconds} s</strong></article>
            <article><span>Statistiques Docker</span><strong>{settings.data.dockerCollectIntervalSeconds} s</strong></article>
            <article><span>Découverte Docker</span><strong>{settings.data.dockerDiscoveryIntervalSeconds} s</strong></article>
            <article><span>Rétention métriques</span><strong>{settings.data.metricsRetentionDays} jours</strong></article>
          </div>
          <article className="chart-card"><h2>État des collecteurs</h2>
            <p>Système : {settings.data.collectors.system.status}</p>
            <p>Docker : {settings.data.collectors.docker.status}</p>
            <small>Les valeurs sont définies par la configuration locale du service et ne sont pas modifiables depuis l'interface.</small>
          </article>
        </>
      )}
    </section>
  )
}
