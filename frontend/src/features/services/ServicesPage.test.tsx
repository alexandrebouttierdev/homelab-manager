import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { SystemUnit, UnitDetail, UnitInventory } from '../../lib/api/services'
import { getLastUnitAction, getUnit, listUnits, requestUnitAction } from '../../lib/api/services'
import { RefreshProvider } from '../../lib/refresh'
import { ServicesPage } from './ServicesPage'

vi.mock('../../lib/api/services', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/api/services')>()),
  listUnits: vi.fn(),
  getUnit: vi.fn(),
  requestUnitAction: vi.fn(),
  getLastUnitAction: vi.fn(),
}))

const session = vi.hoisted(() => ({ role: 'viewer' as 'viewer' | 'operator' | 'admin' }))
vi.mock('../../hooks/useSession', () => ({
  sessionQueryKey: ['session'],
  useSession: () => ({
    user: { id: 'u', username: 'olivia', displayName: 'Olivia', role: session.role },
    isPending: false,
    unauthenticated: false,
  }),
}))

function unit(name: string, activeState: string, overrides: Partial<SystemUnit> = {}): SystemUnit {
  return {
    name,
    description: `Description de ${name}`,
    type: 'service',
    loadState: 'loaded',
    activeState,
    subState: activeState === 'active' ? 'running' : 'failed',
    unitFileState: 'enabled',
    stateChangedAt: '2026-11-23T09:00:00Z',
    mainPid: null,
    memoryBytes: null,
    cpuNanoseconds: null,
    result: null,
    nextElapse: null,
    lastTrigger: null,
    triggers: [],
    ...overrides,
  }
}

const inventory: UnitInventory = {
  status: 'ok',
  collectedAt: '2026-11-23T10:00:00Z',
  counts: { total: 3, active: 2, failed: 1, activeTimers: 1 },
  units: [
    unit('ssh.service', 'active', { mainPid: 812, memoryBytes: 4 * 1024 * 1024 }),
    unit('backup.timer', 'active', { type: 'timer', triggers: ['backup.service'] }),
    unit('backup.service', 'failed', { result: 'exit-code' }),
  ],
  manageableUnits: [],
}

const detail: UnitDetail = {
  unit: inventory.units[0],
  definition: {
    fragmentPath: '/lib/systemd/system/ssh.service',
    execStart: ['/usr/sbin/sshd -D'],
    requires: ['sysinit.target'],
    after: ['network.target'],
    restart: 'on-failure',
    slice: 'system.slice',
    tasksMax: { value: null, unlimited: true },
    limitNofile: { value: null, unlimited: false },
    memoryMax: { value: 536_870_912, unlimited: false },
  },
  journal: {
    available: true,
    entries: [
      {
        cursor: 'c2',
        time: '2026-11-23T09:02:00Z',
        priority: 6,
        unit: 'ssh.service',
        identifier: 'sshd',
        pid: '812',
        hostname: 'homelab',
        bootId: 'b',
        message: 'newest line',
        truncated: false,
      },
      {
        cursor: 'c1',
        time: '2026-11-23T09:01:00Z',
        priority: 6,
        unit: 'ssh.service',
        identifier: 'sshd',
        pid: '812',
        hostname: 'homelab',
        bootId: 'b',
        message: 'oldest line',
        truncated: false,
      },
    ],
  },
}

function renderPage(path = '/services') {
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <RefreshProvider>
        <MemoryRouter initialEntries={[path]}>
          <ServicesPage />
        </MemoryRouter>
      </RefreshProvider>
    </QueryClientProvider>,
  )
}

function unitNames() {
  return screen
    .getAllByRole('row')
    .slice(1)
    .map((row) => within(row).getAllByRole('button')[0].textContent ?? '')
}

describe('ServicesPage', () => {
  beforeEach(() => {
    session.role = 'viewer'
  })
  afterEach(() => vi.clearAllMocks())

  it('lists the units failed first, filters them and opens a detail', async () => {
    vi.mocked(listUnits).mockResolvedValue(inventory)
    vi.mocked(getUnit).mockResolvedValue(detail)
    const user = userEvent.setup()
    renderPage()

    expect(await screen.findByText('À jour')).toBeInTheDocument()
    expect(screen.getByText('Timers actifs')).toBeInTheDocument()
    expect(unitNames()[0]).toMatch(/^backup\.service/)
    expect(screen.getByText('812')).toBeInTheDocument()
    expect(screen.getByText('4 Mo')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /En échec/ }))
    expect(unitNames()).toHaveLength(1)
    await user.click(screen.getByRole('button', { name: /^Tous/ }))
    await user.selectOptions(screen.getByLabelText('Filtrer par type'), 'timer')
    expect(unitNames()).toEqual([expect.stringMatching(/^backup\.timer/)])
    await user.selectOptions(screen.getByLabelText('Filtrer par type'), 'all')
    await user.type(screen.getByLabelText('Rechercher une unité par son nom'), 'ssh')
    expect(unitNames()).toHaveLength(1)

    await user.click(screen.getByRole('button', { name: /^ssh\.service/ }))
    expect(await screen.findByText('/usr/sbin/sshd -D')).toBeInTheDocument()
    expect(getUnit).toHaveBeenCalledWith('ssh.service')
    expect(screen.getByText('illimité')).toBeInTheDocument()
    expect(screen.getByText('inconnu')).toBeInTheDocument()
    expect(screen.getByText('512 Mo')).toBeInTheDocument()
    const lines = screen.getAllByRole('listitem').map((item) => item.textContent ?? '')
    const oldest = lines.findIndex((line) => line.includes('oldest line'))
    expect(oldest).toBeGreaterThanOrEqual(0)
    expect(oldest).toBeLessThan(lines.findIndex((line) => line.includes('newest line')))

    await user.click(screen.getByRole('button', { name: 'Fermer le détail' }))
    expect(screen.queryByText('/usr/sbin/sshd -D')).not.toBeInTheDocument()
  })

  it('opens the unit named in the address, as linked from an event', async () => {
    vi.mocked(listUnits).mockResolvedValue(inventory)
    vi.mocked(getUnit).mockResolvedValue(detail)
    renderPage('/services?unit=ssh.service')
    expect(await screen.findByText('/lib/systemd/system/ssh.service')).toBeInTheDocument()
  })

  it('tells that systemd cannot be reached', async () => {
    vi.mocked(listUnits).mockResolvedValue({
      status: 'unavailable',
      collectedAt: null,
      counts: { total: 0, active: 0, failed: 0, activeTimers: 0 },
      units: [],
      manageableUnits: [],
    })
    renderPage()
    expect(await screen.findByText(/systemd n'est pas joignable/)).toBeInTheDocument()
    expect(screen.getByText('systemd indisponible')).toBeInTheDocument()
    expect(screen.queryByText("Aucune unité pour l'instant")).not.toBeInTheDocument()
    expect(screen.queryByText('Timers actifs')).not.toBeInTheDocument()
  })

  it('lets an operator restart a unit of the allow list after confirmation', async () => {
    session.role = 'operator'
    vi.mocked(listUnits).mockResolvedValue({ ...inventory, manageableUnits: ['ssh.service'] })
    vi.mocked(getUnit).mockResolvedValue(detail)
    const running = {
      unit: 'ssh.service',
      action: 'restart' as const,
      state: 'running' as const,
      failure: null,
      requestedBy: 'olivia',
      requestedAt: '2026-11-24T10:00:00Z',
      finishedAt: null,
      final: null,
    }
    vi.mocked(requestUnitAction).mockResolvedValue(running)
    vi.mocked(getLastUnitAction).mockResolvedValue({
      ...running,
      state: 'succeeded',
      finishedAt: '2026-11-24T10:00:03Z',
      final: { activeState: 'active', subState: 'running' },
    })
    const user = userEvent.setup()
    renderPage('/services?unit=ssh.service')

    await user.click(await screen.findByRole('button', { name: 'Redémarrer' }))
    expect(screen.getByText(/Redémarrer ssh\.service \?/)).toBeInTheDocument()
    expect(requestUnitAction).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Confirmer : Redémarrer' }))
    await waitFor(() => expect(requestUnitAction).toHaveBeenCalledWith('ssh.service', 'restart'))
    expect(await screen.findByText('ssh.service redémarrée (état : active).')).toBeInTheDocument()
    expect(getLastUnitAction).toHaveBeenCalledWith('ssh.service')
  })

  it('shows no action to a viewer', async () => {
    vi.mocked(listUnits).mockResolvedValue({ ...inventory, manageableUnits: ['ssh.service'] })
    vi.mocked(getUnit).mockResolvedValue(detail)
    renderPage('/services?unit=ssh.service')
    expect(await screen.findByText('/usr/sbin/sshd -D')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Redémarrer' })).not.toBeInTheDocument()
  })

  it('shows no action on a unit outside the allow list', async () => {
    session.role = 'operator'
    vi.mocked(listUnits).mockResolvedValue(inventory)
    vi.mocked(getUnit).mockResolvedValue(detail)
    renderPage('/services?unit=ssh.service')
    expect(await screen.findByText('/usr/sbin/sshd -D')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Redémarrer' })).not.toBeInTheDocument()
  })
})
