import { z } from 'zod'

export const retentionSchema = z.object({
  metricsRetentionDays: z
    .number({ error: 'Nombre de jours requis' })
    .int('Nombre entier de jours')
    .min(1, 'Au moins 1 jour')
    .max(365, 'Au plus 365 jours'),
})

export type RetentionValues = z.infer<typeof retentionSchema>

/**
 * Measured on ~100 host series, indexes included: ~40 bytes per raw point,
 * kept one day, and ~58 bytes per 5-minute rollup, kept the whole retention
 * (docs/METRICS_COLLECTION.md § 4).
 */
const seriesPerCycle = 100
const bytesPerPoint = 40
const bytesPerRollup = 58
const rawDays = 1
const rollupsPerDay = 288

/** Estimated steady-state size of the host history, in megabytes. */
export function estimatedStorageMegabytes(days: number, systemIntervalSeconds: number): number {
  if (days <= 0 || systemIntervalSeconds <= 0) return 0
  const cyclesPerDay = 86_400 / systemIntervalSeconds
  const raw = Math.min(days, rawDays) * cyclesPerDay * seriesPerCycle * bytesPerPoint
  const rollups = days * rollupsPerDay * seriesPerCycle * bytesPerRollup
  return (raw + rollups) / 1e6
}
