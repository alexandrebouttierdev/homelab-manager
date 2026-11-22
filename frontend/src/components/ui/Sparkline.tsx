import { useId } from 'react'
import { cn } from '../../lib/cn'

type SparklineTone = 'cpu' | 'ram' | 'net' | 'accent' | 'success'

const colors: Record<SparklineTone, string> = {
  cpu: 'var(--app-cpu)',
  ram: 'var(--app-ram)',
  net: 'var(--app-net)',
  accent: 'var(--app-accent)',
  success: 'var(--app-success)',
}

type SparklineProps = {
  values: number[]
  tone?: SparklineTone
  className?: string
}

export function Sparkline({ values, tone = 'accent', className }: SparklineProps) {
  const gradientId = `spark-${useId().replace(/:/g, '')}`
  if (values.length < 2) return null

  const width = 100
  const height = 32
  const minimum = Math.min(...values)
  const maximum = Math.max(...values)
  const span = maximum - minimum || 1
  const points = values.map((value, index) => {
    const x = (index / (values.length - 1)) * width
    const y = height - 3 - ((value - minimum) / span) * (height - 6)
    return [x, y] as const
  })
  const line = points.map(([x, y]) => `${x.toFixed(2)},${y.toFixed(2)}`).join(' ')
  const area = `0,${height} ${line} ${width},${height}`
  const color = colors[tone]

  return (
    <svg
      className={cn('h-9 w-full overflow-visible', className)}
      viewBox={`0 0 ${width} ${height}`}
      preserveAspectRatio="none"
      aria-hidden="true"
    >
      <defs>
        <linearGradient id={gradientId} x1="0" x2="0" y1="0" y2="1">
          <stop offset="0%" stopColor={color} stopOpacity="0.18" />
          <stop offset="100%" stopColor={color} stopOpacity="0" />
        </linearGradient>
      </defs>
      <polygon fill={`url(#${gradientId})`} points={area} />
      <polyline
        fill="none"
        points={line}
        stroke={color}
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.5"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  )
}
