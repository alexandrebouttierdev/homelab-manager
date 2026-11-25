import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { getCron, inspectCron, type CronInventory } from '../../lib/api/cron'
import { RefreshProvider } from '../../lib/refresh'
import { CronPage } from './CronPage'

vi.mock('../../lib/api/cron', () => ({ getCron: vi.fn(), inspectCron: vi.fn() }))

const inventory: CronInventory = {
  timeZone: 'CET',
  entries: [
    {
      source: '/etc/crontab',
      line: 7,
      expression: '17 * * * *',
      user: 'root',
      command: 'cd / && run-parts --report /etc/cron.hourly',
      valid: true,
      error: null,
      description: 'Toutes les heures, à la minute 17',
      nextRuns: ['2026-11-24T10:17:00Z'],
      lastRun: '2026-11-24T09:17:01Z',
    },
    {
      source: '/etc/cron.d/backup',
      line: 2,
      expression: '61 * * * *',
      user: 'root',
      command: '/opt/backup.sh token=••••',
      valid: false,
      error: 'invalid cron expression: minute "61"',
      description: null,
      nextRuns: [],
      lastRun: null,
    },
  ],
  directories: [
    { period: 'daily', path: '/etc/cron.daily', scripts: ['dpkg', 'logrotate'], scheduledBy: null },
  ],
  anacron: true,
  unreadable: [],
  timers: [
    {
      name: 'apt-daily.timer',
      description: 'Daily apt download activities',
      activeState: 'active',
      nextElapse: '2026-11-25T06:00:00Z',
      lastTrigger: '2026-11-24T06:00:00Z',
      service: 'apt-daily.service',
      serviceResult: 'exit-code',
    },
  ],
  timersAvailable: true,
  journalAvailable: false,
}

function renderPage() {
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <RefreshProvider>
        <MemoryRouter>
          <CronPage />
        </MemoryRouter>
      </RefreshProvider>
    </QueryClientProvider>,
  )
}

describe('CronPage', () => {
  afterEach(() => vi.clearAllMocks())

  it('lists the Cron tasks, the run-parts directories and the timers, read-only', async () => {
    vi.mocked(getCron).mockResolvedValue(inventory)
    const user = userEvent.setup()
    renderPage()
    expect(await screen.findByText('Toutes les heures, à la minute 17')).toBeInTheDocument()
    expect(screen.getByText('Expression invalide')).toBeInTheDocument()
    expect(screen.getByText('/opt/backup.sh token=••••')).toBeInTheDocument()
    expect(
      screen.getByText(/dernières exécutions ne peuvent pas être affichées/),
    ).toBeInTheDocument()
    expect(screen.getByText('Par anacron, qui rattrape un passage manqué')).toBeInTheDocument()
    expect(screen.getByText('dpkg, logrotate')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /Lancer|Modifier|Supprimer/ }),
    ).not.toBeInTheDocument()

    await user.type(screen.getByLabelText('Rechercher une tâche Cron'), 'backup')
    expect(screen.queryByText('Toutes les heures, à la minute 17')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /Timers systemd/ }))
    expect(screen.getByText('apt-daily.timer')).toBeInTheDocument()
    expect(screen.getByText('Code de sortie non nul')).toBeInTheDocument()
  })

  it('inspects an expression without saving it', async () => {
    vi.mocked(getCron).mockResolvedValue(inventory)
    vi.mocked(inspectCron).mockResolvedValue({
      description: 'Du lundi au vendredi à 09:00',
      nextRuns: ['2026-11-25T08:00:00Z', '2026-11-26T08:00:00Z'],
    })
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('button', { name: 'Inspecteur' }))
    const input = screen.getByLabelText(/Expression Cron/)
    await user.clear(input)
    await user.click(screen.getByRole('button', { name: 'Calculer' }))
    expect(await screen.findByText('Expression requise')).toBeInTheDocument()
    expect(inspectCron).not.toHaveBeenCalled()
    await user.type(input, '0 9 * * 1-5')
    await user.click(screen.getByRole('button', { name: 'Calculer' }))
    await waitFor(() => expect(inspectCron).toHaveBeenCalledWith('0 9 * * 1-5'))
    expect(await screen.findByText('Du lundi au vendredi à 09:00')).toBeInTheDocument()
  })
})
