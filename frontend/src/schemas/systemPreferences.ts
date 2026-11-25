import { z } from 'zod'

const label = '[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?'

// Mirror of PUT /api/v1/system/preferences: it guides the input, the server
// and the systemd services decide.
export const systemPreferencesSchema = z.object({
  hostname: z
    .string()
    .trim()
    .min(1, 'Nom requis')
    .max(64, 'Au plus 64 caractères')
    .regex(
      new RegExp(`^${label}(\\.${label})*$`),
      'Lettres, chiffres et tirets, sans tiret au début ni à la fin ; points entre les parties',
    ),
  timezone: z
    .string()
    .trim()
    .max(64, 'Au plus 64 caractères')
    .regex(
      /^[A-Za-z0-9_+-]+(\/[A-Za-z0-9_+-]+){0,3}$/,
      'Fuseau de la base tz, par exemple Europe/Paris',
    ),
  locale: z
    .string()
    .trim()
    .max(64, 'Au plus 64 caractères')
    .regex(
      /^([a-z]{2,3}(_[A-Z]{2})?(\.[A-Za-z0-9-]{1,20})?(@[a-z]{1,20})?|C(\.(UTF-8|utf8))?|POSIX)$/,
      'Langue comme fr_FR.UTF-8, C.UTF-8 ou POSIX',
    ),
})

export type SystemPreferencesValues = z.infer<typeof systemPreferencesSchema>
