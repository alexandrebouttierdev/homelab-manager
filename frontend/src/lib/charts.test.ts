import { describe, expect, it } from 'vitest'
import { buildChartOption } from '../features/containers/useContainerDetailPage'
import { axisFormatter } from '../features/metrics/useMetricsPage'
import { axisTooltip, formatTooltipTime, timeAxis, timePoints, tooltipValue } from './charts'
import { formatPercent } from './format'

const theme = {
  text: 't',
  muted: 'm',
  grid: 'g',
  axis: 'a',
  tooltipBackground: 'tb',
  tooltipBorder: 'tbo',
  tooltipText: 'tt',
  cpu: 'c',
  ram: 'r',
  net: 'n',
  accent: 'ac',
  danger: 'd',
  warning: 'w',
}

type AxisPointerFormatter = (params: { value: unknown }) => string
type ValueFormatter = (value: unknown, dataIndex: number) => string

describe('chart helpers', () => {
  it('formats the tooltip header as a local French date and time', () => {
    // Local constructor: the expected text does not depend on the time zone.
    const local = new Date(2026, 10, 25, 20, 25, 37, 402)
    expect(formatTooltipTime(local)).toBe('25/11/2026 20:25:37')
    expect(formatTooltipTime(local.getTime())).toBe('25/11/2026 20:25:37')
    expect(formatTooltipTime(local.toISOString())).toBe('25/11/2026 20:25:37')
    expect(formatTooltipTime('not a date')).toBe('—')
  })

  it('turns samples into sorted time points', () => {
    expect(
      timePoints([
        { recordedAt: '2026-11-25T19:26:00Z', value: 2 },
        { recordedAt: 'invalid', value: 9 },
        { recordedAt: '2026-11-25T19:25:37.402472579Z', value: 1 },
      ]),
    ).toEqual([
      [Date.parse('2026-11-25T19:25:37.402Z'), 1],
      [Date.parse('2026-11-25T19:26:00Z'), 2],
    ])
  })

  it('reads the value of a tooltip row, bare or as a time point', () => {
    expect(tooltipValue(7.3310659898477155)).toBe(7.3310659898477155)
    expect(tooltipValue([1_764_098_737_402, 2.48])).toBe(2.48)
    expect(tooltipValue(null)).toBeNull()
    expect(tooltipValue('-')).toBeNull()
  })

  it('formats the tooltip header and values instead of the raw data', () => {
    const tooltip = axisTooltip(theme, (value) => formatPercent(value))
    const axisPointer = tooltip.axisPointer as { label: { formatter: AxisPointerFormatter } }
    const valueFormatter = tooltip.valueFormatter as ValueFormatter
    const local = new Date(2026, 10, 25, 20, 25, 37)

    expect(axisPointer.label.formatter({ value: local.getTime() })).toBe('25/11/2026 20:25:37')
    expect(axisPointer.label.formatter({ value: local.toISOString() })).toBe('25/11/2026 20:25:37')
    expect(valueFormatter(7.3310659898477155, 0)).toBe('7,3 %')
    expect(valueFormatter([local.getTime(), 2.480022022779614], 0)).toBe('2,5 %')
    expect(valueFormatter(null, 0)).toBe('—')
  })

  it('uses a time axis whose labels never repeat the same minute', () => {
    const axis = timeAxis(theme) as { type: string; axisLabel: { formatter: object } }
    expect(axis.type).toBe('time')
    expect(axis.axisLabel.formatter).toMatchObject({
      hour: '{HH}:{mm}',
      minute: '{HH}:{mm}',
      second: '{HH}:{mm}:{ss}',
      day: '{dd}/{MM}',
    })
  })

  it('plots container samples at their real time', () => {
    const option = buildChartOption(
      [
        {
          name: 'cpu.percent',
          recordedAt: '2026-11-25T19:25:47Z',
          value: 5,
          unit: 'percent',
          labels: {},
        },
      ],
      [{ recordedAt: '2026-11-25T19:25:37Z', value: 2 }],
      theme,
    )
    const series = option.series as { name: string; data: unknown }[]
    expect((option.xAxis as { type: string }).type).toBe('time')
    expect(series.map((item) => item.data)).toEqual([
      [[Date.parse('2026-11-25T19:25:47Z'), 5]],
      [[Date.parse('2026-11-25T19:25:37Z'), 2]],
    ])
  })

  it('formats the metrics y-axis ticks with French decimals', () => {
    expect(axisFormatter('percent')(0.5)).toBe('0,5 %')
    expect(axisFormatter('celsius')(42.25)).toBe('42,25 °C')
    expect(axisFormatter('count')(12000)).toBe('12\u202f000')
  })
})
