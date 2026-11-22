import type { BadgeTone } from '../components/ui/Badge'

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
    case 'collector.unavailable':
      return 'Collecteur indisponible'
    default:
      return type
  }
}
