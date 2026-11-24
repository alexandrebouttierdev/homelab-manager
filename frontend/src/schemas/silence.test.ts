import { describe, expect, it } from 'vitest'
import { silenceSchema } from './silence'

describe('silenceSchema', () => {
  it('accepts a preset duration and a reason', () => {
    expect(
      silenceSchema.safeParse({ scope: 'target', durationMinutes: '60', reason: ' Migration ' })
        .data?.reason,
    ).toBe('Migration')
  })

  it('refuses a missing, long or multi-line reason and an unknown duration', () => {
    for (const values of [
      { scope: 'all', durationMinutes: '60', reason: '  ' },
      { scope: 'all', durationMinutes: '60', reason: 'a'.repeat(201) },
      { scope: 'all', durationMinutes: '60', reason: 'a\nb' },
      { scope: 'all', durationMinutes: '5', reason: 'x' },
      { scope: 'host', durationMinutes: '60', reason: 'x' },
    ]) {
      expect(silenceSchema.safeParse(values).success).toBe(false)
    }
  })
})
