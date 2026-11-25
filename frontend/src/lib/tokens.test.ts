import { describe, expect, it } from 'vitest'
import type { ApiToken } from './api/tokens'
import { tokenExpiry, tokenScopeLabel, tokenScopeOptions } from './tokens'

const token: ApiToken = {
  id: 't',
  name: 'ha',
  prefix: 'hlm_abcdefgh',
  scopes: ['metrics:read'],
  username: 'vera',
  createdAt: '2026-11-24T10:00:00Z',
  expiresAt: '2026-12-01T10:00:00Z',
  lastUsedAt: null,
}

describe('tokens', () => {
  it('labels the scopes and tells the expiry', () => {
    expect(tokenScopeOptions('viewer').map((option) => option.value)).toEqual([
      'metrics:read',
      'containers:read',
      'events:read',
      'services:read',
      'backups:read',
    ])
    expect(tokenScopeOptions('operator').filter((option) => option.action)).toEqual([
      { value: 'services:operate', label: 'Agir sur les services', action: true },
      { value: 'containers:operate', label: 'Agir sur les conteneurs', action: true },
    ])
    expect(tokenScopeLabel('backups:read')).toBe('Sauvegardes')
    expect(tokenExpiry(token, new Date('2026-11-24T10:00:00Z'))).toEqual({
      label: 'Expire dans 7 j',
      tone: 'warning',
    })
    expect(tokenExpiry(token, new Date('2026-11-01T10:00:00Z')).tone).toBe('neutral')
    expect(tokenExpiry(token, new Date('2026-12-02T10:00:00Z'))).toEqual({
      label: 'Expiré',
      tone: 'danger',
    })
  })
})
