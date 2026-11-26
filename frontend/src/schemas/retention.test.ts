import { describe, expect, it } from 'vitest'
import { estimatedStorageMegabytes, retentionSchema } from './retention'

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

describe('estimatedStorageMegabytes', () => {
  it('counts one day of raw points and the rollups of the whole retention', () => {
    expect(estimatedStorageMegabytes(1, 10)).toBeCloseTo(36.2, 1)
    expect(estimatedStorageMegabytes(7, 10)).toBeCloseTo(46.3, 1)
    expect(estimatedStorageMegabytes(7, 30)).toBeCloseTo(23.2, 1)
    expect(estimatedStorageMegabytes(365, 10)).toBeCloseTo(644.3, 1)
  })

  it('is zero for invalid inputs', () => {
    expect(estimatedStorageMegabytes(0, 10)).toBe(0)
    expect(estimatedStorageMegabytes(7, 0)).toBe(0)
  })
})
