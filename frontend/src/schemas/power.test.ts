import { describe, expect, it } from 'vitest'
import { powerConfirmationSchema } from './power'

describe('powerConfirmationSchema', () => {
  const schema = powerConfirmationSchema('homelab')

  it('accepts the exact host name, ignoring surrounding spaces', () => {
    expect(schema.safeParse({ confirmHostname: 'homelab' }).success).toBe(true)
    expect(schema.safeParse({ confirmHostname: ' homelab ' }).success).toBe(true)
  })

  it.each(['', 'Homelab', 'homelab.local', 'other'])('rejects %j', (value) => {
    expect(schema.safeParse({ confirmHostname: value }).success).toBe(false)
  })

  it('never accepts anything while the host name is unknown', () => {
    expect(powerConfirmationSchema('').safeParse({ confirmHostname: '' }).success).toBe(false)
  })
})
