import { describe, expect, it } from 'vitest'
import type { MetricSample } from './api/monitoring'
import { filesystemSummary, interfaceSummary } from './host'

const at = '2026-11-22T21:00:00Z'
const gb = 1e9

function fs(
  mountpoint: string,
  device: string | undefined,
  total: number,
  used: number,
): MetricSample[] {
  const labels: Record<string, string> = device ? { mountpoint, device } : { mountpoint }
  return [
    { name: 'filesystem.total', value: total, unit: 'bytes', labels, recordedAt: at },
    { name: 'filesystem.used', value: used, unit: 'bytes', labels, recordedAt: at },
    { name: 'filesystem.available', value: total - used, unit: 'bytes', labels, recordedAt: at },
    {
      name: 'filesystem.percent',
      value: (used / total) * 100,
      unit: 'percent',
      labels,
      recordedAt: at,
    },
  ]
}

describe('filesystemSummary', () => {
  it('counts a volume mounted several times only once (btrfs subvolumes)', () => {
    const summary = filesystemSummary([
      ...fs('/', '/dev/nvme0n1p3', 1000 * gb, 50 * gb),
      ...fs('/home', '/dev/nvme0n1p3', 1000 * gb, 50 * gb),
      ...fs('/boot', '/dev/nvme0n1p2', 2 * gb, 1 * gb),
    ])
    expect(summary.entries).toHaveLength(3)
    expect(summary.volumes).toBe(2)
    expect(summary.total).toBe(1002 * gb)
    expect(summary.used).toBe(51 * gb)
  })

  it('falls back to identical sizes when the device label is missing', () => {
    const summary = filesystemSummary([
      ...fs('/', undefined, 1000 * gb, 50 * gb),
      ...fs('/home', undefined, 1000 * gb, 50 * gb),
    ])
    expect(summary.volumes).toBe(1)
    expect(summary.total).toBe(1000 * gb)
  })

  it('keeps distinct volumes of the same size apart when devices differ', () => {
    const summary = filesystemSummary([
      ...fs('/data1', '/dev/sda1', 500 * gb, 10 * gb),
      ...fs('/data2', '/dev/sdb1', 500 * gb, 10 * gb),
    ])
    expect(summary.volumes).toBe(2)
    expect(summary.total).toBe(1000 * gb)
  })
})

describe('interfaceSummary', () => {
  it('keeps errors and drops separate', () => {
    const labels = { interface: 'wlan0' }
    const [wlan] = interfaceSummary([
      { name: 'network.rx_bytes', value: 10, unit: 'bytes', labels, recordedAt: at },
      { name: 'network.errors', value: 0, unit: 'count', labels, recordedAt: at },
      { name: 'network.drops', value: 24837, unit: 'count', labels, recordedAt: at },
    ])
    expect(wlan.errors).toBe(0)
    expect(wlan.drops).toBe(24837)
  })
})
