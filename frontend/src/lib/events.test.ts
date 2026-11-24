import { describe, expect, it } from 'vitest'
import type { MonitorEvent } from './api/monitoring'
import { eventTarget, eventTone, eventTypeLabel } from './events'

function event(overrides: Partial<MonitorEvent>): MonitorEvent {
  return {
    id: 'e',
    containerId: null,
    type: 'service.failed',
    severity: 'error',
    message: 'Unit entered the failed state',
    occurredAt: '2026-11-23T10:00:00Z',
    ...overrides,
  }
}

describe('events', () => {
  it('names the service events', () => {
    expect(eventTypeLabel('service.failed')).toBe('Unité en échec')
    expect(eventTypeLabel('service.recovered')).toBe('Unité rétablie')
    expect(eventTone('error')).toBe('danger')
  })

  it('links a service event to its unit', () => {
    expect(eventTarget(event({ metadata: { unit: 'getty@tty1.service' } }))).toEqual({
      kind: 'unit',
      to: '/services?unit=getty%40tty1.service',
      label: 'getty@tty1.service',
    })
  })

  it('links a container event to its container', () => {
    expect(eventTarget(event({ type: 'container.removed', containerId: 'abc' }))?.to).toBe(
      '/containers/abc',
    )
  })

  it('has no target without a container or a unit', () => {
    expect(eventTarget(event({ metadata: { unit: 42 } }))).toBeNull()
    expect(eventTarget(event({ type: 'container.removed' }))).toBeNull()
  })
})
