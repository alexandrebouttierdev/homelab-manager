import { describe, expect, it } from 'vitest'
import { accountSchema } from './account'

const valid = { username: 'alice', displayName: '', role: 'viewer', password: 'initial-password-1' }

describe('accountSchema', () => {
  it('accepts a valid account with an empty display name', () => {
    expect(accountSchema.safeParse(valid).success).toBe(true)
  })

  it.each([
    { ...valid, username: 'Alice' },
    { ...valid, username: '' },
    { ...valid, username: 'a'.repeat(65) },
    { ...valid, role: 'root' },
    { ...valid, password: 'short' },
    { ...valid, displayName: 'x'.repeat(129) },
  ])('rejects %o', (values) => {
    expect(accountSchema.safeParse(values).success).toBe(false)
  })
})
