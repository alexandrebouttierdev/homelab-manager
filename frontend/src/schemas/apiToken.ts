import { z } from 'zod'

// Mirror of POST /api/v1/auth/tokens: it guides the input, the server decides.
export const apiTokenSchema = z.object({
  name: z.string().trim().min(1, 'Nom requis').max(100, 'Au plus 100 caractères'),
  scopes: z
    .array(
      z.enum([
        'metrics:read',
        'containers:read',
        'events:read',
        'services:read',
        'backups:read',
        'services:operate',
        'containers:operate',
      ]),
    )
    .min(1, 'Choisissez au moins une portée'),
  expiresInDays: z
    .number({ error: 'Durée requise' })
    .int('Nombre entier de jours')
    .min(1, 'Au moins 1 jour')
    .max(365, 'Au plus 365 jours'),
})

export type ApiTokenValues = z.infer<typeof apiTokenSchema>
