import { useQuery } from '@tanstack/react-query'
import { listEvents } from '../../lib/api/monitoring'

export function EventsPage() {
  const events = useQuery({ queryKey: ['events'], queryFn: listEvents, refetchInterval: 30_000 })
  return (
    <section className="content">
      <header><div><p className="eyebrow">Dernières 24 heures</p><h1>Événements</h1></div></header>
      {events.isLoading && <p>Chargement des événements...</p>}
      {events.isError && <p className="field-error" role="alert">{events.error.message}</p>}
      {!events.isLoading && !events.isError && events.data?.length === 0 && (
        <article className="chart-card"><h2>Aucun événement enregistré</h2><p>Les changements d'état et incidents de collecte apparaîtront ici.</p></article>
      )}
      {events.data && events.data.length > 0 && (
        <div className="event-list">
          {events.data.map((event) => (
            <article className="chart-card" key={event.id}>
              <div className="event-heading"><strong>{event.type}</strong><span>{event.severity}</span></div>
              <p>{event.message}</p>
              <small>{new Date(event.occurredAt).toLocaleString('fr-FR')}</small>
            </article>
          ))}
        </div>
      )}
    </section>
  )
}
