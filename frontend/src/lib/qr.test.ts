import { describe, expect, it } from 'vitest'
import { qrPath, qrViewBox } from './qr'

describe('qrPath', () => {
  it('draws one square per dark module, shifted by the quiet zone', () => {
    expect(qrPath(['10', '01'])).toBe('M4 4h1v1h-1zM5 5h1v1h-1z')
    expect(qrPath(['00'])).toBe('')
  })

  it('leaves a quiet zone of four modules around the code', () => {
    expect(qrViewBox(21)).toBe('0 0 29 29')
  })
})
