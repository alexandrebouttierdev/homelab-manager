import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { listEventsPage, type MonitorEvent } from '../../lib/api/monitoring'
import { RefreshProvider } from '../../lib/refresh'
import { EventsPage } from './EventsPage'

vi.mock('../../lib/api/monitoring', () => ({ listEventsPage: vi.fn() }))

function event(id: string, message: string): MonitorEvent {
  return {
    id,
    containerId: null,
    type: 'container.state_changed',
    severity: 'info',
    message,
    occurredAt: '2026-11-25T08:00:00Z',
  }
}

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <RefreshProvider>
        <MemoryRouter>
          <EventsPage />
        </MemoryRouter>
      </RefreshProvider>
    </QueryClientProvider>,
  )
}

describe('EventsPage', () => {
  beforeEach(() => {
    vi.mocked(listEventsPage).mockReset()
  })

  it('loads the older events of the same window after the cursor', async () => {
    vi.mocked(listEventsPage)
      .mockResolvedValueOnce({ events: [event('1', 'Premier événement')], nextCursor: 'next' })
      .mockResolvedValueOnce({ events: [event('2', 'Événement plus ancien')], nextCursor: null })
    renderPage()

    expect(await screen.findByText('Premier événement')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Charger plus' }))
    expect(await screen.findByText('Événement plus ancien')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Charger plus' })).not.toBeInTheDocument()

    const [first, second] = vi.mocked(listEventsPage).mock.calls.map(([options]) => options)
    expect(first.cursor).toBeUndefined()
    expect(second.cursor).toBe('next')
    expect(second.from).toEqual(first.from)
    expect(second.to).toEqual(first.to)
  })
})
