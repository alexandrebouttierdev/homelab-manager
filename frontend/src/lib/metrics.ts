import type { MetricSample } from './api/monitoring'

export type LabelMatcher = Record<string, string>

function labelsMatch(sample: MetricSample, labels?: LabelMatcher): boolean {
  if (!labels) return true
  return Object.entries(labels).every(([key, value]) => sample.labels?.[key] === value)
}

export function sampleKey(sample: MetricSample): string {
  const labels = Object.entries(sample.labels ?? {})
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([key, value]) => `${key}=${value}`)
    .join(',')
  return `${sample.name}|${labels}`
}

export function latestSamples(samples: MetricSample[]): MetricSample[] {
  const latest = new Map<string, MetricSample>()
  for (const sample of samples) {
    const key = sampleKey(sample)
    const current = latest.get(key)
    if (!current || sample.recordedAt > current.recordedAt) {
      latest.set(key, sample)
    }
  }
  return Array.from(latest.values())
}

export function latestOne(
  samples: MetricSample[],
  name: string,
  labels?: LabelMatcher,
): MetricSample | undefined {
  let result: MetricSample | undefined
  for (const sample of samples) {
    if (sample.name !== name || !labelsMatch(sample, labels)) continue
    if (!result || sample.recordedAt > result.recordedAt) result = sample
  }
  return result
}

export function latestGrouped(
  samples: MetricSample[],
  name: string,
  labelKey: string,
): Map<string, MetricSample> {
  const grouped = new Map<string, MetricSample>()
  for (const sample of latestSamples(samples)) {
    if (sample.name !== name) continue
    const key = sample.labels?.[labelKey] ?? 'global'
    grouped.set(key, sample)
  }
  return grouped
}

export function series(
  samples: MetricSample[],
  name: string,
  labels?: LabelMatcher,
): MetricSample[] {
  return samples
    .filter((sample) => sample.name === name && labelsMatch(sample, labels))
    .sort((left, right) => left.recordedAt.localeCompare(right.recordedAt))
}

export function seriesByLabel(
  samples: MetricSample[],
  name: string,
  labelKey: string,
): Map<string, MetricSample[]> {
  const grouped = new Map<string, MetricSample[]>()
  for (const sample of samples) {
    if (sample.name !== name) continue
    const key = sample.labels?.[labelKey] ?? 'global'
    const values = grouped.get(key) ?? []
    values.push(sample)
    grouped.set(key, values)
  }
  for (const values of grouped.values()) {
    values.sort((left, right) => left.recordedAt.localeCompare(right.recordedAt))
  }
  return grouped
}

export function memoryPercentSeries(
  used: MetricSample[],
  total: MetricSample[],
): Array<{ recordedAt: string; value: number }> {
  const totals = new Map(total.map((sample) => [sample.recordedAt, sample.value]))
  return used
    .map((sample) => {
      const totalValue = totals.get(sample.recordedAt)
      if (!totalValue || totalValue <= 0) return null
      return { recordedAt: sample.recordedAt, value: (sample.value / totalValue) * 100 }
    })
    .filter((sample): sample is { recordedAt: string; value: number } => sample !== null)
    .sort((left, right) => left.recordedAt.localeCompare(right.recordedAt))
}

export function average(values: number[]): number | null {
  if (values.length === 0) return null
  return values.reduce((sum, value) => sum + value, 0) / values.length
}

export function latestValue(samples: MetricSample[], name: string, labels?: LabelMatcher): number | null {
  return latestOne(samples, name, labels)?.value ?? null
}
