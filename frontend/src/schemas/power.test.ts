import { describe, expect, it } from 'vitest'
import { powerConfirmationSchema } from './power'

describe('powerConfirmationSchema', () => {
  const schema = powerConfirmationSchema('monoops')

  it('accepts the exact host name, ignoring surrounding spaces', () => {
    expect(schema.safeParse({ confirmHostname: 'monoops' }).success).toBe(true)
    expect(schema.safeParse({ confirmHostname: ' monoops ' }).success).toBe(true)
  })

  it.each(['', 'Monoops', 'monoops.local', 'other'])('rejects %j', (value) => {
    expect(schema.safeParse({ confirmHostname: value }).success).toBe(false)
  })

  it('never accepts anything while the host name is unknown', () => {
    expect(powerConfirmationSchema('').safeParse({ confirmHostname: '' }).success).toBe(false)
  })
})
