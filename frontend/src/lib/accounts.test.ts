import { describe, expect, it } from 'vitest'
import type { Account } from './api/users'
import { toAccountRow } from './accounts'

const base: Account = {
  id: 'a',
  username: 'alice',
  displayName: 'Alice',
  role: 'operator',
  disabled: false,
  mustChangePassword: false,
  lastLoginAt: '2026-11-23T10:00:00Z',
  lastLoginIp: '192.0.2.4',
  createdAt: '2026-11-20T10:00:00Z',
}

describe('toAccountRow', () => {
  it('describes an active account', () => {
    const row = toAccountRow(base, 'b')
    expect(row.roleLabel).toBe('Opérateur')
    expect(row.statusLabel).toBe('Actif')
    expect(row.statusTone).toBe('success')
    expect(row.toggleLabel).toBe('Désactiver')
    expect(row.lastLogin).toContain('192.0.2.4')
    expect(row.isSelf).toBe(false)
  })

  it('flags the pending password change and the current account', () => {
    const row = toAccountRow(
      { ...base, mustChangePassword: true, lastLoginAt: null, lastLoginIp: null },
      'a',
    )
    expect(row.statusLabel).toBe('Mot de passe à changer')
    expect(row.statusTone).toBe('warning')
    expect(row.lastLogin).toBe('Jamais')
    expect(row.isSelf).toBe(true)
  })

  it('puts the disabled state first', () => {
    const row = toAccountRow({ ...base, disabled: true, mustChangePassword: true }, undefined)
    expect(row.statusLabel).toBe('Désactivé')
    expect(row.statusTone).toBe('neutral')
    expect(row.toggleLabel).toBe('Réactiver')
  })
})
