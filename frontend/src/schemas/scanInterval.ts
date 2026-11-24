import { z } from 'zod'

// Bounds of PUT /api/v1/settings/vulnerability-scans (API_CONTRACTS.md § 4.9).
export const scanIntervalSchema = z.object({
  intervalHours: z
    .number({ error: "Nombre d'heures requis" })
    .int("Nombre entier d'heures")
    .min(6, 'Au moins 6 heures')
    .max(168, 'Au plus 168 heures (7 jours)'),
})

export type ScanIntervalValues = z.infer<typeof scanIntervalSchema>
