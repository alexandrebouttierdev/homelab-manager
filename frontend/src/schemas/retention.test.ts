import { describe, expect, it } from 'vitest'
import { estimatedStorageGigabytes, retentionSchema } from './retention'

describe('retentionSchema', () => {
  it('accepts whole days between 1 and 365', () => {
    expect(retentionSchema.safeParse({ metricsRetentionDays: 7 }).success).toBe(true)
    expect(retentionSchema.safeParse({ metricsRetentionDays: 365 }).success).toBe(true)
  })

  it('rejects out-of-range or fractional values', () => {
    for (const value of [0, 366, 2.5, Number.NaN]) {
      expect(retentionSchema.safeParse({ metricsRetentionDays: value }).success).toBe(false)
    }
  })
})

describe('estimatedStorageGigabytes', () => {
  it('matches the measured ~300 MB per day at 10 s', () => {
    expect(estimatedStorageGigabytes(1, 10)).toBeCloseTo(0.294, 2)
    expect(estimatedStorageGigabytes(7, 10)).toBeCloseTo(2.06, 1)
    expect(estimatedStorageGigabytes(7, 30)).toBeCloseTo(0.69, 1)
  })
})
