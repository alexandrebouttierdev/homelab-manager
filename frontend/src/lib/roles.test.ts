import { describe, expect, it } from 'vitest'
import {
  canManagePower,
  canManageSettings,
  canManageUsers,
  isRole,
  roleLabel,
  roleOptions,
} from './roles'

describe('roles', () => {
  it('labels every role in French, in increasing order of rights', () => {
    expect(roleOptions).toEqual([
      { value: 'viewer', label: 'Lecteur' },
      { value: 'operator', label: 'Opérateur' },
      { value: 'admin', label: 'Administrateur' },
    ])
    expect(roleLabel('admin')).toBe('Administrateur')
  })

  it('recognises only the defined roles', () => {
    expect(isRole('operator')).toBe(true)
    expect(isRole('root')).toBe(false)
    expect(isRole('toString')).toBe(false)
  })

  it('mirrors the server policy for display only', () => {
    expect(canManageUsers('admin')).toBe(true)
    expect(canManageUsers('operator')).toBe(false)
    expect(canManageUsers(undefined)).toBe(false)
    expect(canManageSettings('admin')).toBe(true)
    expect(canManageSettings('viewer')).toBe(false)
    expect(canManagePower('admin')).toBe(true)
    expect(canManagePower('operator')).toBe(false)
  })
})
