import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render } from '@testing-library/react'
import { screen } from '@testing-library/dom'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { RefreshProvider } from '../../lib/refresh'
import { ThemeProvider } from '../../lib/theme'
import { DashboardPage } from './DashboardPage'

const recordedAt = '2026-11-21T09:00:00Z'

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    json: async () => body,
  })
}

function dashboardResponse() {
  return {
    host: {
      hostname: 'nas',
      os: { distribution: 'Debian', version: '13' },
      kernelVersion: '6.12',
      bootTime: '2026-11-21T08:00:00Z',
      uptimeSeconds: 3600,
      collector: { status: 'ok', lastSuccessAt: recordedAt },
    },
    containers: { total: 0, running: 0, stopped: 0, other: 0 },
    metrics: [],
    collectors: {
      system: { status: 'ok', lastSuccessAt: recordedAt },
      docker: { status: 'ok', lastSuccessAt: recordedAt },
    },
  }
}

function currentMetricsResponse() {
  return {
    status: 'ok',
    lastSuccessAt: recordedAt,
    samples: [
      { name: 'cpu.percent', value: 21.4, unit: 'percent', labels: { core: 'all' }, recordedAt },
      { name: 'cpu.cores', value: 4, unit: 'count', labels: null, recordedAt },
      { name: 'cpu.load1', value: 0.42, unit: 'processes', labels: null, recordedAt },
      { name: 'memory.used', value: 1073741824, unit: 'bytes', labels: null, recordedAt },
      { name: 'memory.total', value: 2147483648, unit: 'bytes', labels: null, recordedAt },
      { name: 'memory.available', value: 1073741824, unit: 'bytes', labels: null, recordedAt },
      { name: 'filesystem.total', value: 1000000000, unit: 'bytes', labels: { mountpoint: '/' }, recordedAt },
      { name: 'filesystem.used', value: 400000000, unit: 'bytes', labels: { mountpoint: '/' }, recordedAt },
      { name: 'filesystem.percent', value: 40, unit: 'percent', labels: { mountpoint: '/' }, recordedAt },
      { name: 'network.rx_bytes', value: 2048, unit: 'bytes', labels: { interface: 'eth0' }, recordedAt },
      { name: 'network.tx_bytes', value: 1024, unit: 'bytes', labels: { interface: 'eth0' }, recordedAt },
    ],
  }
}

function historyResponse() {
  return {
    samples: [
      { name: 'cpu.percent', value: 18.2, unit: 'percent', labels: { core: 'all' }, recordedAt: '2026-11-21T08:30:00Z' },
      { name: 'cpu.percent', value: 21.4, unit: 'percent', labels: { core: 'all' }, recordedAt },
      { name: 'memory.used', value: 1000000000, unit: 'bytes', labels: null, recordedAt: '2026-11-21T08:30:00Z' },
      { name: 'memory.used', value: 1073741824, unit: 'bytes', labels: null, recordedAt },
      { name: 'memory.total', value: 2147483648, unit: 'bytes', labels: null, recordedAt: '2026-11-21T08:30:00Z' },
      { name: 'memory.total', value: 2147483648, unit: 'bytes', labels: null, recordedAt },
    ],
  }
}

describe('DashboardPage', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('renders host metrics and collector status from the API', async () => {
    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL) => {
      const url = String(input)
      if (url.startsWith('/api/v1/dashboard')) return jsonResponse(dashboardResponse())
      if (url.startsWith('/api/v1/host/metrics/current')) return jsonResponse(currentMetricsResponse())
      if (url.startsWith('/api/v1/containers')) return jsonResponse({ containers: [] })
      if (url.startsWith('/api/v1/events')) return jsonResponse({ events: [] })
      if (url.startsWith('/api/v1/host/metrics/')) return jsonResponse(historyResponse())
      return jsonResponse({})
    }))

    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })

    render(
      <ThemeProvider>
        <QueryClientProvider client={client}>
          <RefreshProvider>
            <MemoryRouter>
              <DashboardPage />
            </MemoryRouter>
          </RefreshProvider>
        </QueryClientProvider>
      </ThemeProvider>,
    )

    expect(await screen.findByRole('heading', { name: "Vue d'ensemble" })).toBeInTheDocument()
    expect(await screen.findByText('Statut général : optimal')).toBeInTheDocument()
    expect(screen.getAllByText('nas').length).toBeGreaterThan(0)
    expect(screen.getAllByText('21,4 %').length).toBeGreaterThan(0)
    expect(screen.getAllByText('1 Go').length).toBeGreaterThan(0)
    expect(screen.getByText('Système')).toBeInTheDocument()
    expect(screen.getByText('Docker')).toBeInTheDocument()
  })
})
