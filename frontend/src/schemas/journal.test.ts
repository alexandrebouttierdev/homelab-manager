import { describe, expect, it } from 'vitest'
import { journalSearchSchema, rangeBound } from './journal'

describe('journalSearchSchema', () => {
  it('accepts a search and a range in order', () => {
    const result = journalSearchSchema.safeParse({
      q: '  sshd  ',
      from: '2026-11-23T09:00',
      to: '2026-11-23T10:00',
    })
    expect(result.success && result.data.q).toBe('sshd')
  })

  it('accepts empty values', () => {
    expect(journalSearchSchema.safeParse({ q: '', from: '', to: '' }).success).toBe(true)
  })

  it('refuses a search too long or a range out of order', () => {
    expect(journalSearchSchema.safeParse({ q: 'x'.repeat(201), from: '', to: '' }).success).toBe(
      false,
    )
    const result = journalSearchSchema.safeParse({
      q: '',
      from: '2026-11-23T10:00',
      to: '2026-11-23T10:00',
    })
    expect(result.success).toBe(false)
    expect(result.error?.issues[0].path).toEqual(['to'])
  })

  it('turns a datetime-local value into RFC 3339', () => {
    expect(rangeBound('')).toBeUndefined()
    expect(rangeBound('2026-11-23T10:00')).toBe(new Date('2026-11-23T10:00').toISOString())
  })
})
