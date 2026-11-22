import type { BadgeTone } from '../components/ui/Badge'

/** Collector states of the API (docs/API_CONTRACTS.md § 4.1). */
export type CollectorState = 'starting' | 'ok' | 'stale' | 'unavailable'

const labels: Record<CollectorState, string> = {
  starting: 'Démarrage',
  ok: 'Opérationnel',
  stale: 'Données périmées',
  unavailable: 'Indisponible',
}

const tones: Record<CollectorState, BadgeTone> = {
  starting: 'info',
  ok: 'success',
  stale: 'warning',
  unavailable: 'danger',
}

function known(status: string): status is CollectorState {
  return status in labels
}

export function collectorStatusLabel(status: string): string {
  return known(status) ? labels[status] : status
}

export function collectorStatusTone(status: string): BadgeTone {
  return known(status) ? tones[status] : 'neutral'
}
