import { lazy, Suspense } from 'react'
import type { EChartsOption } from 'echarts'
import { cn } from '../../lib/cn'

const ECharts = lazy(() => import('./MetricsChart'))

type ChartProps = {
  option: EChartsOption
  height?: number
  className?: string
  label?: string
}

export function Chart({ option, height = 280, className, label = 'Graphique' }: ChartProps) {
  return (
    <div className={cn('relative w-full', className)} style={{ minHeight: height }} aria-label={label}>
      <Suspense
        fallback={(
          <div
            className="w-full animate-pulse rounded-md border border-line/60 bg-elevated/40"
            style={{ height }}
            aria-hidden="true"
          />
        )}
      >
        <ECharts option={option} height={height} />
      </Suspense>
    </div>
  )
}
