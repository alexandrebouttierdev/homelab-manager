import type { BadgeTone } from '../components/ui/Badge'
import type { MonitorEvent } from './api/monitoring'

export function eventTone(severity: string | null | undefined): BadgeTone {
  switch (severity) {
    case 'error':
    case 'critical':
      return 'danger'
    case 'warning':
      return 'warning'
    case 'info':
      return 'info'
    default:
      return 'neutral'
  }
}

export function eventSeverityLabel(severity: string | null | undefined): string {
  switch (severity) {
    case 'error':
      return 'Erreur'
    case 'critical':
      return 'Critique'
    case 'warning':
      return 'Avertissement'
    case 'info':
      return 'Information'
    default:
      return severity || 'Neutre'
  }
}

export function eventTypeLabel(type: string): string {
  switch (type) {
    case 'container.started':
      return 'Conteneur démarré'
    case 'container.stopped':
      return 'Conteneur arrêté'
    case 'container.restarted':
      return 'Conteneur redémarré'
    case 'container.created':
      return 'Conteneur créé'
    case 'container.removed':
      return 'Conteneur supprimé'
    case 'container.died':
      return 'Conteneur hors service'
    case 'container.discovered':
      return 'Conteneur découvert'
    case 'container.state_changed':
      return 'État du conteneur modifié'
    case 'service.failed':
      return 'Unité en échec'
    case 'service.recovered':
      return 'Unité rétablie'
    case 'collector.unavailable':
      return 'Collecteur indisponible'
    default:
      return type
  }
}

/** What an event is about: a container, a systemd unit, or nothing to open. */
export type EventTarget =
  | { kind: 'container'; to: string; label: string }
  | { kind: 'unit'; to: string; label: string }
  | null

export function eventTarget(event: MonitorEvent): EventTarget {
  if (event.containerId) {
    return {
      kind: 'container',
      to: `/containers/${encodeURIComponent(event.containerId)}`,
      label: 'ouvrir le conteneur',
    }
  }
  const unit = event.metadata?.unit
  if (event.type.startsWith('service.') && typeof unit === 'string' && unit !== '') {
    return {
      kind: 'unit',
      to: `/services?${new URLSearchParams({ unit }).toString()}`,
      label: unit,
    }
  }
  return null
}
