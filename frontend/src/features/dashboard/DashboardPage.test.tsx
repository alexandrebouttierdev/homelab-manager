import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DashboardPage } from './DashboardPage'

describe('DashboardPage', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('renders host metrics and collector status from the API', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        host: {
          hostname: 'nas',
          os: { distribution: 'Debian', version: '13' },
          kernelVersion: '6.12',
          bootTime: '2026-11-21T08:00:00Z',
          uptimeSeconds: 3600,
          collector: { status: 'ok', lastSuccessAt: '2026-11-21T09:00:00Z' },
        },
        containers: { total: 2, running: 1, stopped: 1, other: 0 },
        metrics: [
          { name: 'cpu.percent', value: 21.4, unit: 'percent', labels: { core: 'all' }, recordedAt: '2026-11-21T09:00:00Z' },
          { name: 'memory.used', value: 1073741824, unit: 'bytes', labels: null, recordedAt: '2026-11-21T09:00:00Z' },
          { name: 'memory.total', value: 2147483648, unit: 'bytes', labels: null, recordedAt: '2026-11-21T09:00:00Z' },
        ],
        collectors: {
          system: { status: 'ok', lastSuccessAt: '2026-11-21T09:00:00Z' },
          docker: { status: 'ok', lastSuccessAt: '2026-11-21T09:00:00Z' },
        },
      }),
    }))
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })

    render(
      <QueryClientProvider client={client}>
        <DashboardPage />
      </QueryClientProvider>,
    )

    expect(await screen.findByText('nas')).toBeInTheDocument()
    expect(screen.getByText('21.4 %')).toBeInTheDocument()
    expect(screen.getByText('1.0 Go')).toBeInTheDocument()
    expect(screen.getByText('Collectes opérationnelles')).toBeInTheDocument()
  })
})
