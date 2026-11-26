import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { JournalEntry, JournalPage as Page } from '../../lib/api/journal'
import {
  getJournalEntry,
  getJournalSummary,
  listJournal,
  listJournalBoots,
  listJournalUnits,
} from '../../lib/api/journal'
import { ThemeProvider } from '../../lib/theme'
import { JournalPage } from './JournalPage'

vi.mock('../../lib/api/journal', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/api/journal')>()),
  listJournal: vi.fn(),
  getJournalEntry: vi.fn(),
  getJournalSummary: vi.fn(),
  listJournalBoots: vi.fn(),
  listJournalUnits: vi.fn(),
}))
vi.mock('../../components/charts/Chart', () => ({ Chart: () => <div data-testid="chart" /> }))

function entry(n: number, overrides: Partial<JournalEntry> = {}): JournalEntry {
  return {
    cursor: `cursor-${n}`,
    time: `2026-11-23T09:${String(n).padStart(2, '0')}:00Z`,
    priority: 6,
    unit: 'ssh.service',
    identifier: 'sshd',
    pid: '42',
    hostname: 'monoops',
    bootId: 'b',
    message: `message ${n}`,
    truncated: false,
    ...overrides,
  }
}

function page(entries: JournalEntry[], older: string | null, newer: string | null): Page {
  return {
    entries,
    olderCursor: older,
    newerCursor: newer,
    scanned: entries.length,
    scanLimitReached: false,
    skipped: 0,
  }
}

function renderPage() {
  render(
    <ThemeProvider>
      <QueryClientProvider
        client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
      >
        <MemoryRouter>
          <JournalPage />
        </MemoryRouter>
      </QueryClientProvider>
    </ThemeProvider>,
  )
}

describe('JournalPage', () => {
  beforeEach(() => {
    vi.mocked(listJournal).mockImplementation((_filter, request = {}) => {
      if (request.before) return Promise.resolve(page([entry(1)], null, null))
      if (request.after)
        return Promise.resolve(page([entry(9, { message: 'live entry' })], null, 'cursor-9'))
      return Promise.resolve(page([entry(3, { priority: 3 }), entry(2)], 'cursor-2', 'cursor-3'))
    })
    vi.mocked(getJournalEntry).mockResolvedValue({
      entry: entry(3, { priority: 3 }),
      fields: [
        { name: 'MESSAGE', value: 'message 3', masked: false },
        { name: '_CMDLINE', value: '[masqué]', masked: true },
      ],
    })
    vi.mocked(getJournalSummary).mockResolvedValue({
      start: '2026-11-22T09:15:00Z',
      bucketMinutes: 15,
      buckets: [{ start: '2026-11-22T09:15:00Z', counts: [0, 0, 0, 2, 1, 0, 5, 0] }],
      total: 8,
      truncated: false,
      diskUsageBytes: 8 * 1024 * 1024,
      fullAccess: false,
      generatedAt: '2026-11-23T09:10:00Z',
    })
    vi.mocked(listJournalBoots).mockResolvedValue([])
    vi.mocked(listJournalUnits).mockResolvedValue(['cron.service', 'ssh.service'])
  })
  afterEach(() => vi.clearAllMocks())

  it('lists entries, inspects one and loads older ones', async () => {
    const user = userEvent.setup()
    renderPage()

    expect(await screen.findByText('message 3')).toBeInTheDocument()
    expect(screen.getByText('Erreur')).toBeInTheDocument()
    expect(screen.getByText(/ne lit que ses propres entrées/)).toBeInTheDocument()
    expect(screen.getByText('8 entrées')).toBeInTheDocument()

    await user.click(screen.getAllByRole('button', { name: /Inspecter l'entrée/ })[0])
    expect(await screen.findByText('[masqué]')).toBeInTheDocument()
    expect(getJournalEntry).toHaveBeenCalledWith('cursor-3')

    await user.click(screen.getByRole('button', { name: 'Charger les entrées plus anciennes' }))
    expect(await screen.findByText('message 1')).toBeInTheDocument()
    expect(vi.mocked(listJournal).mock.calls.some((call) => call[1]?.before === 'cursor-2')).toBe(
      true,
    )
    expect(
      screen.queryByRole('button', { name: 'Charger les entrées plus anciennes' }),
    ).not.toBeInTheDocument()
  })

  it('follows newer entries from the newest cursor', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('message 3')

    await user.click(screen.getByRole('button', { name: 'Suivre en direct' }))
    expect(await screen.findByText('live entry')).toBeInTheDocument()
    expect(vi.mocked(listJournal).mock.calls.some((call) => call[1]?.after === 'cursor-3')).toBe(
      true,
    )
    const rows = within(screen.getByRole('table')).getAllByRole('row')
    expect(rows[1]).toHaveTextContent('live entry')
    expect(screen.getByRole('button', { name: 'Suivi en direct' })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
  })

  it('reads the journal again when a filter changes', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('message 3')

    await user.selectOptions(screen.getByLabelText('Priorité'), 'err')
    await waitFor(() =>
      expect(vi.mocked(listJournal).mock.calls.some((call) => call[0].priority === 'err')).toBe(
        true,
      ),
    )
    await user.selectOptions(screen.getByLabelText('Source'), 'docker')
    expect(screen.getByLabelText('Unité')).toBeDisabled()

    await user.type(screen.getByLabelText('Texte recherché'), 'accepted')
    await user.click(screen.getByRole('button', { name: 'Appliquer' }))
    await waitFor(() =>
      expect(vi.mocked(listJournal).mock.calls.some((call) => call[0].q === 'accepted')).toBe(true),
    )
    // The default period, the last 24 hours, adds its start.
    expect(screen.getByRole('link', { name: '.log' }).getAttribute('href')).toMatch(
      /^\/api\/v1\/journal\/export\?source=docker&priority=err&from=[^&]+&q=accepted&format=log$/,
    )
  })
})
