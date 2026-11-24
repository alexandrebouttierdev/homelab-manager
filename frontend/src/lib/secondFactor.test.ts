import { describe, expect, it } from 'vitest'
import { formatTotpSecret, otherMethods, preferredMethod } from './secondFactor'

describe('formatTotpSecret', () => {
  it('groups the secret by four characters', () => {
    expect(formatTotpSecret('GEZDGNBVGY3TQOJQ')).toBe('GEZD GNBV GY3T QOJQ')
    expect(formatTotpSecret('ABCDE')).toBe('ABCD E')
  })
})

describe('second factor methods', () => {
  it('prefers the key, then the app, then a recovery code', () => {
    expect(preferredMethod(['totp', 'webauthn', 'recovery'])).toBe('webauthn')
    expect(preferredMethod(['recovery', 'totp'])).toBe('totp')
    expect(preferredMethod(['recovery'])).toBe('recovery')
    expect(preferredMethod([])).toBe('recovery')
  })

  it('lists the other methods of the account', () => {
    expect(otherMethods(['totp', 'webauthn', 'recovery'], 'webauthn')).toEqual(['totp', 'recovery'])
    expect(otherMethods(['totp', 'recovery'], 'recovery')).toEqual(['totp'])
  })
})
