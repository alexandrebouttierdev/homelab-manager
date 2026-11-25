import { requestJSON } from './client'

/** Host name, time zone and language of the machine (API_CONTRACTS.md § 4.12). */
export type SystemPreferences = { hostname: string; timezone: string; locale: string }

export type SystemSetting = 'hostname' | 'timezone' | 'locale'

export type SystemPreferencesState = {
  /** system_preferences_enabled: changes are possible. */
  enabled: boolean
  /** False where the distribution forbids systemd to change the language (Debian). */
  localeEditable: boolean
  preferences: SystemPreferences
  /** Time zones the machine knows; empty when the list could not be read. */
  timezones: string[]
  /** Only in the answer of a change. */
  changed?: SystemSetting[]
}

export function getSystemPreferences() {
  return requestJSON<SystemPreferencesState>('/api/v1/system/preferences')
}

export function updateSystemPreferences(preferences: SystemPreferences) {
  return requestJSON<SystemPreferencesState>('/api/v1/system/preferences', {
    method: 'PUT',
    body: JSON.stringify(preferences),
  })
}
