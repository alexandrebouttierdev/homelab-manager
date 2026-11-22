import { describe, expect, it } from 'vitest'
import { labelsText } from './metrics'

describe('labelsText', () => {
  it('joins labels or shows a dash', () => {
    expect(labelsText({ core: '0', host: 'a' })).toBe('core=0 · host=a')
    expect(labelsText(null)).toBe('—')
    expect(labelsText({})).toBe('—')
  })
})
