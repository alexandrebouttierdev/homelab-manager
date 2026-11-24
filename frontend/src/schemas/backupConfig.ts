import { z } from 'zod'

// Mirror of the server rules (SPEC_ADMINISTRATION.md § 7.3, API_CONTRACTS.md
// § 4.10): it guides the input, the server decides.
const absolutePath = /^\/(?!.*\/\/)(?!.*\/$)(?!.*(^|\/)\.\.(\/|$)).*$/
const lines = (text: string) =>
  text
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line !== '')

const keepCount = z
  .number({ error: 'Nombre requis' })
  .int('Nombre entier')
  .min(0, 'Au moins 0')
  .max(1000, 'Au plus 1 000')

export const backupConfigSchema = z
  .object({
    type: z.enum(['none', 'local', 'rest', 's3']),
    location: z.string().trim().max(2048, 'Au plus 2 048 caractères'),
    username: z.string().max(1024, 'Au plus 1 024 caractères'),
    secret: z.string().max(1024, 'Au plus 1 024 caractères'),
    sources: z.string(),
    excludes: z.string(),
    excludeCaches: z.boolean(),
    excludeIfPresent: z
      .string()
      .trim()
      .regex(/^[A-Za-z0-9._-]{0,255}$/, "Un nom de fichier : lettres, chiffres, '.', '_' ou '-'"),
    scheduleEnabled: z.boolean(),
    frequency: z.enum(['daily', 'weekly']),
    weekday: z.number().int().min(0).max(6),
    time: z.string().regex(/^([01]\d|2[0-3]):[0-5]\d$/, 'Heure au format HH:MM'),
    retentionEnabled: z.boolean(),
    keepDaily: keepCount,
    keepWeekly: keepCount,
    keepMonthly: keepCount,
    keepYearly: keepCount,
    checkEnabled: z.boolean(),
    checkSubsetPercent: z
      .number({ error: 'Pourcentage requis' })
      .int('Nombre entier')
      .min(1, 'Au moins 1 %')
      .max(100, 'Au plus 100 %'),
  })
  .superRefine((values, context) => {
    if (values.type === 'local' && (!absolutePath.test(values.location) || values.location === '/'))
      context.addIssue({
        code: 'custom',
        path: ['location'],
        message: 'Chemin absolu, sans « .. » ni « / » final',
      })
    if (
      (values.type === 'rest' || values.type === 's3') &&
      !/^https:\/\/[^/@?#\s]+/.test(values.location)
    )
      context.addIssue({
        code: 'custom',
        path: ['location'],
        message: 'Adresse https://, sans identifiants',
      })
    if (values.type === 's3' && !/^https:\/\/[^/]+\/[^/]+/.test(values.location))
      context.addIssue({
        code: 'custom',
        path: ['location'],
        message: 'Indiquez le bucket : https://hôte/bucket[/préfixe]',
      })
    const sources = lines(values.sources)
    if (sources.length > 50)
      context.addIssue({ code: 'custom', path: ['sources'], message: 'Au plus 50 sources' })
    const invalid = sources.find((source) => !absolutePath.test(source))
    if (invalid)
      context.addIssue({
        code: 'custom',
        path: ['sources'],
        message: `« ${invalid} » n'est pas un chemin absolu propre`,
      })
    if (new Set(sources).size !== sources.length)
      context.addIssue({ code: 'custom', path: ['sources'], message: 'Une source est en double' })
    const excludes = lines(values.excludes)
    if (excludes.length > 500)
      context.addIssue({ code: 'custom', path: ['excludes'], message: 'Au plus 500 motifs' })
    if (
      values.retentionEnabled &&
      values.keepDaily + values.keepWeekly + values.keepMonthly + values.keepYearly === 0
    )
      context.addIssue({
        code: 'custom',
        path: ['keepDaily'],
        message: 'La rétention garde au moins un instantané',
      })
    if (excludes.some((pattern) => pattern.startsWith('#')))
      context.addIssue({
        code: 'custom',
        path: ['excludes'],
        message: 'Un motif ne commence pas par « # »',
      })
  })

export type BackupConfigValues = z.infer<typeof backupConfigSchema>

export { lines as splitLines }
