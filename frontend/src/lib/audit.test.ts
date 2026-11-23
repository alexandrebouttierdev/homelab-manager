import { describe, expect, it } from 'vitest'
import type { AuditEntry } from './api/security'
import { auditPeriodStart, toAuditRow } from './audit'

const entry: AuditEntry = {
  id: 'a1',
  actor: 'alice',
  targetType: 'route',
  targetId: 'GET /api/v1/users',
  action: 'access',
  outcome: 'denied',
  requestId: null,
  createdAt: '2026-11-23T09:00:00Z',
}

describe('toAuditRow', () => {
  it('translates outcomes and keeps raw identifiers', () => {
    const row = toAuditRow(entry)
    expect(row.outcomeLabel).toBe('Refusé')
    expect(row.outcomeTone).toBe('danger')
    expect(row.target).toBe('route · GET /api/v1/users')
    expect(row.requestId).toBe('—')
  })

  it('knows every outcome', () => {
    expect(toAuditRow({ ...entry, outcome: 'success' }).outcomeLabel).toBe('Réussi')
    expect(toAuditRow({ ...entry, outcome: 'failure' }).outcomeTone).toBe('warning')
    expect(toAuditRow({ ...entry, outcome: 'requested' }).outcomeLabel).toBe('Demandé')
    expect(toAuditRow({ ...entry, outcome: 'other' }).outcomeLabel).toBe('other')
  })
})

describe('auditPeriodStart', () => {
  it('computes the start of the period', () => {
    const now = new Date('2026-11-23T10:00:00Z')
    expect(auditPeriodStart('24h', now)?.toISOString()).toBe('2026-11-22T10:00:00.000Z')
    expect(auditPeriodStart('7d', now)?.toISOString()).toBe('2026-11-16T10:00:00.000Z')
    expect(auditPeriodStart('all', now)).toBeUndefined()
  })
})
