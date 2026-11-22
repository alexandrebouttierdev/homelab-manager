import { describe, expect, it } from 'vitest'
import { loginSchema } from './login'

describe('loginSchema', () => {
  it('rejects incomplete credentials', () => {
    expect(loginSchema.safeParse({ username: '', password: 'admin' }).success).toBe(false)
    expect(loginSchema.safeParse({ username: 'admin', password: '' }).success).toBe(false)
  })

  it('accepts the default administrator credentials', () => {
    expect(loginSchema.safeParse({ username: 'admin', password: 'admin' }).success).toBe(true)
  })

  it('rejects oversized passwords', () => {
    expect(loginSchema.safeParse({ username: 'admin', password: 'x'.repeat(1025) }).success).toBe(false)
  })
})
