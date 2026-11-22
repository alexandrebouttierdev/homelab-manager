const decimalFormat = new Intl.NumberFormat('fr-FR', { maximumFractionDigits: 1 })
const integerFormat = new Intl.NumberFormat('fr-FR', { maximumFractionDigits: 0 })
const relativeFormat = new Intl.RelativeTimeFormat('fr-FR', { numeric: 'auto' })
const dateTimeFormat = new Intl.DateTimeFormat('fr-FR', {
  dateStyle: 'medium',
  timeStyle: 'short',
})
const timeFormat = new Intl.DateTimeFormat('fr-FR', {
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
})
const shortTimeFormat = new Intl.DateTimeFormat('fr-FR', {
  hour: '2-digit',
  minute: '2-digit',
})
const chartTimeFormat = new Intl.DateTimeFormat('fr-FR', {
  day: '2-digit',
  month: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
})

export function formatNumber(value: number | null | undefined, digits = 0): string {
  if (value == null || !Number.isFinite(value)) return '—'
  return new Intl.NumberFormat('fr-FR', { maximumFractionDigits: digits }).format(value)
}

export function formatPercent(value: number | null | undefined, digits = 1): string {
  if (value == null || !Number.isFinite(value)) return '—'
  return `${new Intl.NumberFormat('fr-FR', { maximumFractionDigits: digits }).format(value)} %`
}

export function formatBytes(
  value: number | null | undefined,
  options: { perSecond?: boolean; digits?: number } = {},
): string {
  if (value == null || !Number.isFinite(value)) return '—'
  if (value === 0) return options.perSecond ? '0 o/s' : '0 o'

  const units = ['o', 'Ko', 'Mo', 'Go', 'To', 'Po']
  const exponent = Math.min(
    Math.floor(Math.log(Math.abs(value)) / Math.log(1024)),
    units.length - 1,
  )
  const scaled = value / 1024 ** exponent
  const digits = options.digits ?? (scaled >= 100 ? 0 : 1)
  const formatted = new Intl.NumberFormat('fr-FR', {
    maximumFractionDigits: digits,
  }).format(scaled)

  return `${formatted} ${units[exponent]}${options.perSecond ? '/s' : ''}`
}

export function formatRate(value: number | null | undefined): string {
  return formatBytes(value, { perSecond: true })
}

export function formatUptime(seconds: number | null | undefined): string {
  if (seconds == null || !Number.isFinite(seconds) || seconds < 0) return '—'
  const total = Math.floor(seconds)
  const days = Math.floor(total / 86_400)
  const hours = Math.floor((total % 86_400) / 3_600)
  const minutes = Math.floor((total % 3_600) / 60)

  if (days > 0) {
    return `${formatNumber(days)} j ${String(hours).padStart(2, '0')} h ${String(minutes).padStart(2, '0')} min`
  }
  if (hours > 0) return `${hours} h ${String(minutes).padStart(2, '0')} min`
  if (minutes > 0) return `${minutes} min`
  return '< 1 min'
}

export function formatRelativeTime(value: string | null | undefined): string {
  if (!value) return '—'
  const timestamp = new Date(value).getTime()
  if (!Number.isFinite(timestamp)) return '—'
  const deltaSeconds = Math.round((timestamp - Date.now()) / 1_000)
  const absolute = Math.abs(deltaSeconds)

  if (absolute < 45) return 'à l’instant'
  if (absolute < 3_600) return relativeFormat.format(Math.round(deltaSeconds / 60), 'minute')
  if (absolute < 86_400) return relativeFormat.format(Math.round(deltaSeconds / 3_600), 'hour')
  if (absolute < 2_592_000) return relativeFormat.format(Math.round(deltaSeconds / 86_400), 'day')
  if (absolute < 31_536_000)
    return relativeFormat.format(Math.round(deltaSeconds / 2_592_000), 'month')
  return relativeFormat.format(Math.round(deltaSeconds / 31_536_000), 'year')
}

export function formatDateTime(value: string | Date | null | undefined): string {
  if (!value) return '—'
  const date = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return dateTimeFormat.format(date)
}

export function formatTime(value: string | Date | null | undefined): string {
  if (!value) return '—'
  const date = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return timeFormat.format(date)
}

export function formatShortTime(value: string | Date | null | undefined): string {
  if (!value) return '—'
  const date = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return shortTimeFormat.format(date)
}

export function formatChartTime(value: string | Date): string {
  const date = value instanceof Date ? value : new Date(value)
  return chartTimeFormat.format(date)
}

export function formatCount(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return '—'
  return integerFormat.format(value)
}

export function formatDecimal(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return '—'
  return decimalFormat.format(value)
}

export function formatMetricValue(
  value: number | null | undefined,
  unit: string | null | undefined,
): string {
  if (value == null || !Number.isFinite(value)) return '—'
  switch (unit) {
    case 'percent':
      return formatPercent(value)
    case 'bytes':
      return formatBytes(value)
    case 'bytes_per_second':
      return formatRate(value)
    case 'celsius':
      return `${formatDecimal(value)} °C`
    case 'milliseconds':
      return `${formatDecimal(value)} ms`
    case 'iops':
      return `${formatDecimal(value)} IOPS`
    case 'processes':
      return formatDecimal(value)
    case 'count':
    case 'packets':
    case 'present':
      return formatNumber(value)
    default:
      return unit ? `${formatDecimal(value)} ${unit}` : formatDecimal(value)
  }
}
