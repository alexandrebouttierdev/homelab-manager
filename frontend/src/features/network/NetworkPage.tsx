import { useQuery } from '@tanstack/react-query'
import { getHostMetrics } from '../../lib/api/monitoring'

export function NetworkPage() {
  const received = useQuery({
    queryKey: ['network', 'rx'],
    queryFn: () => getHostMetrics('network.rx_bytes', new Date(Date.now() - 5 * 60 * 1000)),
    refetchInterval: 30_000,
  })
  const sent = useQuery({
    queryKey: ['network', 'tx'],
    queryFn: () => getHostMetrics('network.tx_bytes', new Date(Date.now() - 5 * 60 * 1000)),
    refetchInterval: 30_000,
  })
  const interfaces = new Map<string, { received?: number; sent?: number }>()
  for (const sample of received.data ?? []) {
    const name = sample.labels?.interface
    if (name) interfaces.set(name, { ...interfaces.get(name), received: sample.value })
  }
  for (const sample of sent.data ?? []) {
    const name = sample.labels?.interface
    if (name) interfaces.set(name, { ...interfaces.get(name), sent: sample.value })
  }

  return (
    <section className="content">
      <header><div><p className="eyebrow">Interfaces de l'hôte</p><h1>Réseau</h1></div></header>
      {(received.isError || sent.isError) && <p className="field-error" role="alert">{received.error?.message ?? sent.error?.message}</p>}
      {received.isLoading || sent.isLoading ? <p>Chargement des compteurs réseau...</p> : null}
      {!received.isLoading && !sent.isLoading && interfaces.size === 0 && (
        <article className="chart-card"><h2>Aucune interface mesurée</h2><p>Les compteurs réseau ne sont pas disponibles pour cet hôte.</p></article>
      )}
      {interfaces.size > 0 && (
        <div className="table-scroll">
          <table>
            <thead><tr><th>Interface</th><th>Octets reçus (compteur)</th><th>Octets envoyés (compteur)</th></tr></thead>
            <tbody>{Array.from(interfaces, ([name, counters]) => (
              <tr key={name}><td>{name}</td><td>{counters.received?.toLocaleString('fr-FR') ?? '—'}</td><td>{counters.sent?.toLocaleString('fr-FR') ?? '—'}</td></tr>
            ))}</tbody>
          </table>
        </div>
      )}
    </section>
  )
}
