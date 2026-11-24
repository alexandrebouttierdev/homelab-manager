import { describe, expect, it } from 'vitest'
import { scanIntervalSchema } from './scanInterval'

describe('scanIntervalSchema', () => {
  it('accepts 6 to 168 whole hours', () => {
    for (const hours of [6, 24, 168]) {
      expect(scanIntervalSchema.safeParse({ intervalHours: hours }).success).toBe(true)
    }
    for (const hours of [5, 169, 12.5, Number.NaN]) {
      expect(scanIntervalSchema.safeParse({ intervalHours: hours }).success).toBe(false)
    }
  })
})
