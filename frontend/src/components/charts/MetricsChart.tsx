import * as echarts from 'echarts/core'
import type { EChartsOption } from 'echarts'
import { BarChart, LineChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import ReactEChartsCore from 'echarts-for-react/lib/core'

echarts.use([BarChart, LineChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer])

// Canvas text does not inherit the page font: set it once for every chart.
const textStyle = { fontFamily: "'Instrument Sans Variable', ui-sans-serif, system-ui, sans-serif" }

interface MetricsChartProps {
  option: EChartsOption
  height?: number
}

export default function MetricsChart({ option, height = 280 }: MetricsChartProps) {
  return (
    <ReactEChartsCore
      echarts={echarts}
      option={{ textStyle, ...option }}
      style={{ height, width: '100%' }}
      opts={{ renderer: 'canvas' }}
    />
  )
}
