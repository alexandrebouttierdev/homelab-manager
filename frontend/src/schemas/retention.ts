import { z } from 'zod'

export const retentionSchema = z.object({
  metricsRetentionDays: z
    .number({ error: 'Nombre de jours requis' })
    .int('Nombre entier de jours')
    .min(1, 'Au moins 1 jour')
    .max(365, 'Au plus 365 jours'),
})

export type RetentionValues = z.infer<typeof retentionSchema>

/** Measured average: ~100 host samples per cycle, ~340 bytes per row. */
const samplesPerCycle = 100
const bytesPerSample = 340

/** Estimated steady-state size of the host history, in gigabytes. */
export function estimatedStorageGigabytes(days: number, systemIntervalSeconds: number): number {
  if (days <= 0 || systemIntervalSeconds <= 0) return 0
  const cyclesPerDay = 86_400 / systemIntervalSeconds
  return (days * cyclesPerDay * samplesPerCycle * bytesPerSample) / 1e9
}
