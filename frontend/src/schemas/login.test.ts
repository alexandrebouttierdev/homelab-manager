import { describe, expect, it } from 'vitest'
import { loginSchema } from './login'

describe('loginSchema', () => {
  it('rejects incomplete credentials', () => {
    expect(loginSchema.safeParse({ username: '', password: 'short' }).success).toBe(false)
  })

  it('accepts valid credentials', () => {
    expect(loginSchema.safeParse({ username: 'admin', password: 'secure-password' }).success).toBe(true)
  })
})
