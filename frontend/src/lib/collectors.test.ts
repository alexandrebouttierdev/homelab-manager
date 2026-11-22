import { describe, expect, it } from 'vitest'
import { collectorStatusLabel, collectorStatusTone } from './collectors'

describe('collector states', () => {
  it('translates every API state', () => {
    expect(collectorStatusLabel('ok')).toBe('Opérationnel')
    expect(collectorStatusLabel('stale')).toBe('Données périmées')
    expect(collectorStatusLabel('unavailable')).toBe('Indisponible')
    expect(collectorStatusLabel('starting')).toBe('Démarrage')
    expect(collectorStatusTone('unavailable')).toBe('danger')
    expect(collectorStatusTone('stale')).toBe('warning')
  })

  it('shows an unknown state as is, in a neutral tone', () => {
    expect(collectorStatusLabel('paused')).toBe('paused')
    expect(collectorStatusTone('paused')).toBe('neutral')
  })
})
