import type { SystemPreferences, SystemSetting } from './api/system'

/** Usual languages offered in the list; another installed locale may be typed. */
export const commonLocales = [
  'fr_FR.UTF-8',
  'en_US.UTF-8',
  'en_GB.UTF-8',
  'de_DE.UTF-8',
  'es_ES.UTF-8',
  'it_IT.UTF-8',
  'C.UTF-8',
] as const

const settingLabels: Record<SystemSetting, string> = {
  hostname: 'Nom de la machine',
  timezone: 'Fuseau horaire',
  locale: 'Langue du système',
}

export function systemSettingLabel(setting: SystemSetting): string {
  return settingLabels[setting]
}

/** The changes a submission asks for, in the order the server applies them. */
export function systemChanges(
  current: SystemPreferences,
  values: SystemPreferences,
): Array<{ setting: SystemSetting; from: string; to: string }> {
  return (['hostname', 'timezone', 'locale'] as const)
    .filter((setting) => current[setting] !== values[setting])
    .map((setting) => ({ setting, from: current[setting], to: values[setting] }))
}

/** What each change implies, shown before the confirmation. */
export function systemChangeConsequence(setting: SystemSetting): string {
  switch (setting) {
    case 'hostname':
      return "les adresses qui utilisent l'ancien nom (DNS local, mDNS, certificats TLS) devront suivre ; la confirmation du redémarrage demandera le nouveau nom"
    case 'timezone':
      return "l'heure affichée par la machine et les horaires des tâches Cron changent ; les sauvegardes planifiées suivent l'heure de la machine"
    case 'locale':
      return 'prise en compte par les services à leur prochain démarrage ; les variables LC_* restent inchangées'
  }
}
