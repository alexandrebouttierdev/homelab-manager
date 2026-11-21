import * as echarts from 'echarts/core'
import type { EChartsOption } from 'echarts'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import ReactEChartsCore from 'echarts-for-react/lib/core'

echarts.use([LineChart, GridComponent, TooltipComponent, CanvasRenderer])

interface MetricsChartProps {
  option: EChartsOption
}

export default function MetricsChart({ option }: MetricsChartProps) {
  return <ReactEChartsCore echarts={echarts} option={option} style={{ height: 320 }} />
}
