import { describe, expect, it } from 'vitest'
import { passwordConfirmationSchema, secondFactorLoginSchema, totpCodeSchema } from './secondFactor'

describe('second factor schemas', () => {
  it('accepts a 6-digit code with an optional space', () => {
    expect(totpCodeSchema.safeParse({ code: '123456' }).success).toBe(true)
    expect(totpCodeSchema.safeParse({ code: ' 123 456 ' }).success).toBe(true)
    for (const code of ['12345', '1234567', 'abcdef', '']) {
      expect(totpCodeSchema.safeParse({ code }).success).toBe(false)
    }
  })

  it('checks the code according to the method', () => {
    expect(secondFactorLoginSchema.safeParse({ method: 'totp', code: '123456' }).success).toBe(true)
    expect(
      secondFactorLoginSchema.safeParse({ method: 'totp', code: 'k7pq-2mzx-b9ht' }).success,
    ).toBe(false)
    expect(
      secondFactorLoginSchema.safeParse({ method: 'recovery', code: 'k7pq-2mzx-b9ht' }).success,
    ).toBe(true)
    expect(secondFactorLoginSchema.safeParse({ method: 'recovery', code: '' }).success).toBe(false)
  })

  it('requires the password', () => {
    expect(passwordConfirmationSchema.safeParse({ password: '' }).success).toBe(false)
    expect(passwordConfirmationSchema.safeParse({ password: 'x' }).success).toBe(true)
  })
})
