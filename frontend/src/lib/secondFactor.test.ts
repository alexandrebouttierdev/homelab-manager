import { describe, expect, it } from 'vitest'
import { formatTotpSecret } from './secondFactor'

describe('formatTotpSecret', () => {
  it('groups the secret by four characters', () => {
    expect(formatTotpSecret('GEZDGNBVGY3TQOJQ')).toBe('GEZD GNBV GY3T QOJQ')
    expect(formatTotpSecret('ABCDE')).toBe('ABCD E')
  })
})
