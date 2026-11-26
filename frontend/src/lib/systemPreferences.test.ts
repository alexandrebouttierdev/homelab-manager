import { describe, expect, it } from 'vitest'
import { systemPreferencesSchema } from '../schemas/systemPreferences'
import { systemChangeConsequence, systemChanges } from './systemPreferences'

describe('systemChanges', () => {
  it('lists only what differs, in the order of the server', () => {
    const current = { hostname: 'monoops', timezone: 'Europe/Paris', locale: 'fr_FR.UTF-8' }
    expect(systemChanges(current, current)).toEqual([])
    expect(
      systemChanges(current, { hostname: 'nas', timezone: 'Europe/Paris', locale: 'C.UTF-8' }),
    ).toEqual([
      { setting: 'hostname', from: 'monoops', to: 'nas' },
      { setting: 'locale', from: 'fr_FR.UTF-8', to: 'C.UTF-8' },
    ])
    expect(systemChangeConsequence('hostname')).toMatch(/nouveau nom/)
  })
})

describe('systemPreferencesSchema', () => {
  const valid = {
    hostname: 'nas.home.lan',
    timezone: 'America/Argentina/Buenos_Aires',
    locale: 'en_US.UTF-8',
  }
  it('accepts the usual values', () => {
    expect(systemPreferencesSchema.safeParse(valid).success).toBe(true)
    expect(systemPreferencesSchema.safeParse({ ...valid, locale: 'POSIX' }).success).toBe(true)
  })
  it('refuses the others', () => {
    for (const change of [
      { hostname: 'nas_01' },
      { hostname: 'a'.repeat(65) },
      { timezone: '../etc/passwd' },
      { locale: 'fr_FR.UTF-8;rm' },
    ]) {
      expect(systemPreferencesSchema.safeParse({ ...valid, ...change }).success).toBe(false)
    }
  })
})
