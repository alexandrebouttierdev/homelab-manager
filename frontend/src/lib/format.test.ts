import { describe, expect, it } from 'vitest'
import { formatBytes, formatMetricValue, formatPercent, formatRate, formatUptime } from './format'

describe('format helpers', () => {
  it('formats bytes with French units', () => {
    expect(formatBytes(0)).toBe('0 o')
    expect(formatBytes(1024)).toBe('1 Ko')
    expect(formatBytes(1073741824)).toBe('1 Go')
    expect(formatBytes(null)).toBe('—')
  })

  it('formats rates and percentages', () => {
    expect(formatRate(2048)).toBe('2 Ko/s')
    expect(formatPercent(21.4)).toBe('21,4 %')
    expect(formatPercent(null)).toBe('—')
  })

  it('formats uptime without inventing values', () => {
    expect(formatUptime(59)).toBe('< 1 min')
    expect(formatUptime(3660)).toBe('1 h 01 min')
    expect(formatUptime(90060)).toBe('1 j 01 h 01 min')
    expect(formatUptime(null)).toBe('—')
  })

  it('formats metric values according to their unit', () => {
    expect(formatMetricValue(21.4, 'percent')).toBe('21,4 %')
    expect(formatMetricValue(1536, 'bytes')).toBe('1,5 Ko')
    expect(formatMetricValue(42, 'celsius')).toBe('42 °C')
    expect(formatMetricValue(12.5, 'milliseconds')).toBe('12,5 ms')
    expect(formatMetricValue(3, 'iops')).toBe('3 IOPS')
  })
})
