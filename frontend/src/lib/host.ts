import type { MetricSample } from './api/monitoring'
import { latestGrouped, latestSamples } from './metrics'

export type FilesystemEntry = {
  mountpoint: string
  /** Block device of the volume; several mountpoints may share it. */
  device: string | null
  total: number
  used: number
  available: number
  percent: number | null
}

export type FilesystemSummary = {
  /** One entry per mountpoint. */
  entries: FilesystemEntry[]
  /** Distinct volumes: totals below count each of them once. */
  volumes: number
  total: number
  used: number
  available: number
  percent: number | null
}

export function filesystemSummary(samples: MetricSample[]): FilesystemSummary {
  const totals = latestGrouped(samples, 'filesystem.total', 'mountpoint')
  const used = latestGrouped(samples, 'filesystem.used', 'mountpoint')
  const available = latestGrouped(samples, 'filesystem.available', 'mountpoint')
  const percent = latestGrouped(samples, 'filesystem.percent', 'mountpoint')

  const entries = Array.from(new Set([...totals.keys(), ...used.keys()]))
    .map((mountpoint) => ({
      mountpoint,
      device: totals.get(mountpoint)?.labels?.device ?? null,
      total: totals.get(mountpoint)?.value ?? 0,
      used: used.get(mountpoint)?.value ?? 0,
      available: available.get(mountpoint)?.value ?? 0,
      percent: percent.get(mountpoint)?.value ?? null,
    }))
    .filter((entry) => entry.total > 0)
    .sort((left, right) => right.total - left.total)

  // btrfs subvolumes and bind mounts expose the same volume under several
  // mountpoints. Samples collected before the device label existed fall back
  // to the (total, available) pair, identical for such mountpoints.
  const volumes = new Map<string, FilesystemEntry>()
  for (const entry of entries) {
    const key = entry.device ?? `${entry.total}:${entry.available}`
    if (!volumes.has(key)) volumes.set(key, entry)
  }
  const distinct = Array.from(volumes.values())
  const total = distinct.reduce((sum, entry) => sum + entry.total, 0)
  const usedTotal = distinct.reduce((sum, entry) => sum + entry.used, 0)
  const availableTotal = distinct.reduce((sum, entry) => sum + entry.available, 0)

  return {
    entries,
    volumes: distinct.length,
    total,
    used: usedTotal,
    available: availableTotal,
    percent: total > 0 ? (usedTotal / total) * 100 : null,
  }
}

export type InterfaceEntry = {
  name: string
  rx: number | null
  tx: number | null
  rxRate: number | null
  txRate: number | null
  errors: number
  drops: number
  addresses: string[]
  mac: string | null
}

export function interfaceSummary(samples: MetricSample[]): InterfaceEntry[] {
  const rx = latestGrouped(samples, 'network.rx_bytes', 'interface')
  const tx = latestGrouped(samples, 'network.tx_bytes', 'interface')
  const rxRate = latestGrouped(samples, 'network.rx_bytes_per_second', 'interface')
  const txRate = latestGrouped(samples, 'network.tx_bytes_per_second', 'interface')
  const errors = latestGrouped(samples, 'network.errors', 'interface')
  const drops = latestGrouped(samples, 'network.drops', 'interface')

  const addresses = new Map<string, Set<string>>()
  const macs = new Map<string, string>()
  for (const sample of latestSamples(samples)) {
    if (sample.name !== 'network.interface') continue
    const name = sample.labels?.name
    if (!name) continue
    const address = sample.labels?.address
    if (address) {
      const values = addresses.get(name) ?? new Set<string>()
      values.add(address)
      addresses.set(name, values)
    }
    const mac = sample.labels?.mac
    if (mac) macs.set(name, mac)
  }

  return Array.from(new Set([...rx.keys(), ...tx.keys(), ...addresses.keys()]))
    .map((name) => ({
      name,
      rx: rx.get(name)?.value ?? null,
      tx: tx.get(name)?.value ?? null,
      rxRate: rxRate.get(name)?.value ?? null,
      txRate: txRate.get(name)?.value ?? null,
      errors: errors.get(name)?.value ?? 0,
      drops: drops.get(name)?.value ?? 0,
      addresses: Array.from(addresses.get(name) ?? []),
      mac: macs.get(name) ?? null,
    }))
    .sort((left, right) => (right.rx ?? 0) + (right.tx ?? 0) - ((left.rx ?? 0) + (left.tx ?? 0)))
}

export function diskSummary(samples: MetricSample[]) {
  const readBytes = latestGrouped(samples, 'disk.read_bytes', 'device')
  const writeBytes = latestGrouped(samples, 'disk.write_bytes', 'device')
  const readIops = latestGrouped(samples, 'disk.read_iops', 'device')
  const writeIops = latestGrouped(samples, 'disk.write_iops', 'device')
  const readLatency = latestGrouped(samples, 'disk.read_latency_ms', 'device')
  const writeLatency = latestGrouped(samples, 'disk.write_latency_ms', 'device')

  return Array.from(new Set([...readBytes.keys(), ...writeBytes.keys()]))
    .map((device) => ({
      device,
      readBytes: readBytes.get(device)?.value ?? null,
      writeBytes: writeBytes.get(device)?.value ?? null,
      readIops: readIops.get(device)?.value ?? null,
      writeIops: writeIops.get(device)?.value ?? null,
      readLatency: readLatency.get(device)?.value ?? null,
      writeLatency: writeLatency.get(device)?.value ?? null,
    }))
    .sort((left, right) => left.device.localeCompare(right.device))
}
