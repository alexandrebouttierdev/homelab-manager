import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  listMaintenanceTasks,
  runMaintenanceTask,
  updateMaintenanceTask,
  type MaintenanceTask,
} from '../../lib/api/maintenance'
import { MaintenanceTab } from './MaintenanceTab'

vi.mock('../../lib/api/maintenance', () => ({
  listMaintenanceTasks: vi.fn(),
  updateMaintenanceTask: vi.fn(),
  runMaintenanceTask: vi.fn(),
}))

const session = vi.hoisted(() => ({ admin: true }))
vi.mock('../../hooks/useSession', () => ({
  sessionQueryKey: ['session'],
  useSession: () => ({ canManageSettings: session.admin }),
}))

const task: MaintenanceTask = {
  kind: 'journal-vacuum',
  schedule: { enabled: false, frequency: 'weekly', weekday: 0, time: '04:00' },
  maxSizeMiB: 512,
  sizes: [256, 512, 1024, 2048],
  unit: 'monoops-journal-vacuum@512M.service',
  running: false,
  nextRunAt: null,
  lastRun: {
    at: '2026-11-25T04:00:00Z',
    origin: 'schedule',
    succeeded: false,
    message: 'polkit a refusé',
  },
  updatedAt: null,
  updatedBy: null,
}

function renderTab() {
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <MaintenanceTab />
    </QueryClientProvider>,
  )
}

describe('MaintenanceTab', () => {
  beforeEach(() => {
    session.admin = true
    vi.mocked(listMaintenanceTasks).mockResolvedValue([task])
    vi.mocked(updateMaintenanceTask).mockResolvedValue(task)
    vi.mocked(runMaintenanceTask).mockResolvedValue({ requested: true })
  })
  afterEach(() => vi.clearAllMocks())

  it('schedules the journal cleanup with a size of the list', async () => {
    const user = userEvent.setup()
    renderTab()
    expect(await screen.findByText('Nettoyage du journal système')).toBeInTheDocument()
    expect(screen.getByText('Échec (planifiée) : polkit a refusé')).toBeInTheDocument()
    await user.click(screen.getByLabelText('Planifiée'))
    await user.selectOptions(screen.getByLabelText('Fréquence'), 'daily')
    await user.selectOptions(screen.getByLabelText('Taille maximale du journal'), '1024')
    await user.clear(screen.getByLabelText('Heure'))
    await user.type(screen.getByLabelText('Heure'), '2h')
    expect(screen.getByText('Heure au format HH:MM')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Enregistrer' })).toBeDisabled()
    await user.clear(screen.getByLabelText('Heure'))
    await user.type(screen.getByLabelText('Heure'), '02:30')
    await user.click(screen.getByRole('button', { name: 'Enregistrer' }))
    await waitFor(() =>
      expect(updateMaintenanceTask).toHaveBeenCalledWith('journal-vacuum', {
        schedule: { enabled: true, frequency: 'daily', weekday: 0, time: '02:30' },
        maxSizeMiB: 1024,
      }),
    )
  })

  it('runs a task now', async () => {
    const user = userEvent.setup()
    renderTab()
    await user.click(await screen.findByRole('button', { name: 'Lancer maintenant' }))
    await waitFor(() => expect(runMaintenanceTask).toHaveBeenCalledWith('journal-vacuum'))
  })

  it('is read-only for others than administrators', async () => {
    session.admin = false
    renderTab()
    expect(await screen.findByText('Taille maximale : 512 Mio.')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Lancer maintenant' })).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Fréquence')).not.toBeInTheDocument()
  })
})
