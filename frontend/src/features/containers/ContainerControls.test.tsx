import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  getContainerActionSettings,
  getContainerLogs,
  getLastContainerAction,
  requestContainerAction,
  type ContainerAction,
} from '../../lib/api/containers'
import { ContainerActionPanel, ContainerLogsCard } from './ContainerControls'
import { useContainerAction, useContainerLogs } from './useContainerControls'

vi.mock('../../lib/api/containers', () => ({
  getContainerLogs: vi.fn(),
  getContainerActionSettings: vi.fn(),
  requestContainerAction: vi.fn(),
  getLastContainerAction: vi.fn(),
}))

const session = vi.hoisted(() => ({ role: 'operator' as 'viewer' | 'operator' | 'admin' }))
vi.mock('../../hooks/useSession', () => ({
  sessionQueryKey: ['session'],
  useSession: () => ({
    user: { id: 'u', username: 'olivia', displayName: 'Olivia', role: session.role },
    isPending: false,
    unauthenticated: false,
  }),
}))

function Harness({ state = 'running', removed = false }: { state?: string; removed?: boolean }) {
  const actions = useContainerAction('c-1', 'web', state, removed)
  const logs = useContainerLogs('c-1', removed)
  return (
    <>
      <ContainerActionPanel model={actions} />
      <ContainerLogsCard model={logs} />
    </>
  )
}

function renderHarness(props: { state?: string; removed?: boolean } = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <Harness {...props} />
    </QueryClientProvider>,
  )
}

const running: ContainerAction = {
  containerId: 'c-1',
  name: 'web',
  action: 'restart',
  state: 'running',
  failure: null,
  requestedBy: 'olivia',
  requestedAt: '2026-11-25T10:00:00Z',
  finishedAt: null,
}

describe('container controls', () => {
  beforeEach(() => {
    session.role = 'operator'
    vi.mocked(getContainerActionSettings).mockResolvedValue({
      enabled: true,
      actions: ['start', 'stop', 'restart'],
    })
    vi.mocked(getContainerLogs).mockResolvedValue({
      container: { id: 'c-1', name: 'web' },
      lines: [
        { stream: 'stdout', time: '2026-11-25T08:00:00Z', message: 'listening on :80', cut: false },
        { stream: 'stderr', time: null, message: 'warning: slow disk', cut: true },
      ],
      truncated: false,
      tail: 200,
    })
  })
  afterEach(() => vi.clearAllMocks())

  it('shows the log and filters it', async () => {
    const user = userEvent.setup()
    renderHarness()
    const list = await screen.findByRole('list', { name: 'Lignes du journal' })
    expect(within(list).getByText('listening on :80')).toBeInTheDocument()
    expect(within(list).getByText('[…]')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'stderr' }))
    expect(within(list).queryByText('listening on :80')).not.toBeInTheDocument()
    await user.selectOptions(screen.getByRole('combobox', { name: 'Nombre de lignes' }), '1000')
    await waitFor(() => expect(getContainerLogs).toHaveBeenLastCalledWith('c-1', 1000))
  })

  it('confirms a restart, then follows it', async () => {
    const user = userEvent.setup()
    vi.mocked(requestContainerAction).mockResolvedValue(running)
    vi.mocked(getLastContainerAction).mockResolvedValue({
      ...running,
      state: 'succeeded',
      finishedAt: '2026-11-25T10:00:03Z',
    })
    renderHarness()
    await user.click(await screen.findByRole('button', { name: 'Redémarrer' }))
    expect(screen.getByText(/Redémarrer le conteneur web \?/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Confirmer : Redémarrer' }))
    expect(requestContainerAction).toHaveBeenCalledWith('c-1', 'restart')
    expect(await screen.findByText('web redémarré.')).toBeInTheDocument()
  })

  it('offers Démarrer on a stopped container and nothing to a viewer', async () => {
    renderHarness({ state: 'exited' })
    expect(await screen.findByRole('button', { name: 'Démarrer' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Arrêter' })).not.toBeInTheDocument()
  })

  it('hides the actions from a viewer', async () => {
    session.role = 'viewer'
    renderHarness()
    await screen.findByRole('list', { name: 'Lignes du journal' })
    expect(screen.queryByRole('button', { name: 'Redémarrer' })).not.toBeInTheDocument()
    expect(getContainerActionSettings).not.toHaveBeenCalled()
  })

  it('says when the actions are disabled', async () => {
    vi.mocked(getContainerActionSettings).mockResolvedValue({ enabled: false, actions: [] })
    renderHarness()
    expect(await screen.findByText(/docker_actions_enabled/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Redémarrer' })).not.toBeInTheDocument()
  })

  it('does not read the log of a removed container', async () => {
    renderHarness({ removed: true })
    expect(await screen.findByText('Journal indisponible')).toBeInTheDocument()
    expect(getContainerLogs).not.toHaveBeenCalled()
  })
})
