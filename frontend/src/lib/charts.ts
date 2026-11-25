import type { TooltipComponentOption, XAXisComponentOption } from 'echarts'
import type { useChartTheme } from './theme'

type ChartTheme = ReturnType<typeof useChartTheme>

/** Value of a sample, as a time-axis line series expects it. */
export type TimePoint = [number, number]

const tooltipTimeFormat = new Intl.DateTimeFormat('fr-FR', {
  day: '2-digit',
  month: '2-digit',
  year: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
})

/** Full local date and time of a tooltip header, to the second. */
export function formatTooltipTime(value: string | number | Date): string {
  const date = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return tooltipTimeFormat.format(date)
}

/** Samples as `[timestamp, value]` pairs, sorted by time, for a time axis. */
export function timePoints(samples: Array<{ recordedAt: string; value: number }>): TimePoint[] {
  return samples
    .map((sample): TimePoint => [new Date(sample.recordedAt).getTime(), sample.value])
    .filter(([timestamp]) => Number.isFinite(timestamp))
    .sort((left, right) => left[0] - right[0])
}

/**
 * The value a tooltip row shows: ECharts hands either the bare value or, for
 * a `[time, value]` point, the whole pair.
 */
export function tooltipValue(value: unknown): number | null {
  const raw = Array.isArray(value) ? value[value.length - 1] : value
  return typeof raw === 'number' && Number.isFinite(raw) ? raw : null
}

/**
 * Tooltip of a chart triggered by the axis: the header is the local date and
 * time of the hovered point, each row its value in the series unit.
 */
export function axisTooltip(
  theme: ChartTheme,
  formatValue: (value: number | null) => string,
  options: { shadow?: boolean } = {},
): TooltipComponentOption {
  return {
    trigger: 'axis',
    backgroundColor: theme.tooltipBackground,
    borderColor: theme.tooltipBorder,
    textStyle: { color: theme.tooltipText, fontSize: 11 },
    axisPointer: {
      type: options.shadow ? 'shadow' : 'line',
      label: { formatter: (params) => formatTooltipTime(params.value as string | number) },
    },
    valueFormatter: (value) => formatValue(tooltipValue(value)),
  }
}

/**
 * Time x-axis: ECharts places the ticks on round instants and each label
 * shows only what changes at that level (date at midnight, time otherwise,
 * seconds when the span is short), in the browser's time zone.
 */
export function timeAxis(theme: ChartTheme): XAXisComponentOption {
  return {
    type: 'time',
    boundaryGap: ['0%', '0%'],
    axisTick: { show: false },
    axisLine: { lineStyle: { color: theme.axis } },
    splitLine: { show: false },
    axisLabel: {
      color: theme.muted,
      fontSize: 10,
      hideOverlap: true,
      formatter: {
        year: '{yyyy}',
        month: '{dd}/{MM}',
        day: '{dd}/{MM}',
        hour: '{HH}:{mm}',
        minute: '{HH}:{mm}',
        second: '{HH}:{mm}:{ss}',
        millisecond: '{HH}:{mm}:{ss}',
      },
    },
  }
}
