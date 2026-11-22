import { describe, expect, it } from 'vitest'
import { passwordSchema } from './password'

describe('passwordSchema', () => {
  it('requires at least 12 characters', () => {
    const result = passwordSchema.safeParse({
      currentPassword: 'current-password',
      newPassword: 'short',
      confirmPassword: 'short',
    })

    expect(result.success).toBe(false)
  })

  it('requires matching confirmation', () => {
    const result = passwordSchema.safeParse({
      currentPassword: 'current-password',
      newPassword: 'a-secure-password',
      confirmPassword: 'another-password',
    })

    expect(result.success).toBe(false)
    if (!result.success) {
      expect(result.error.issues[0]?.message).toBe('Les mots de passe ne correspondent pas')
    }
  })

  it('accepts a valid password change', () => {
    const result = passwordSchema.safeParse({
      currentPassword: 'current-password',
      newPassword: 'a-secure-password',
      confirmPassword: 'a-secure-password',
    })

    expect(result.success).toBe(true)
  })
})
