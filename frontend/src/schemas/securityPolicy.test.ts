import { describe, expect, it } from 'vitest'
import { networksFromText, securityPolicySchema } from './securityPolicy'

describe('securityPolicySchema', () => {
  it('accepts the bounds and an empty list', () => {
    expect(securityPolicySchema.safeParse({ idleTimeoutMinutes: 5, networks: '' }).success).toBe(
      true,
    )
    expect(
      securityPolicySchema.safeParse({ idleTimeoutMinutes: 720, networks: '10.0.0.0/8\nfd00::/8' })
        .success,
    ).toBe(true)
  })

  it.each([
    { idleTimeoutMinutes: 4, networks: '' },
    { idleTimeoutMinutes: 721, networks: '' },
    { idleTimeoutMinutes: 60, networks: '10.0.0.1' },
    { idleTimeoutMinutes: 60, networks: 'not-a-network/8' },
  ])('rejects %o', (values) => {
    expect(securityPolicySchema.safeParse(values).success).toBe(false)
  })
})

describe('networksFromText', () => {
  it('splits lines and commas and drops blanks', () => {
    expect(networksFromText(' 10.0.0.0/8 \n\n192.168.1.0/24, fd00::/8 ')).toEqual([
      '10.0.0.0/8',
      '192.168.1.0/24',
      'fd00::/8',
    ])
  })
})
