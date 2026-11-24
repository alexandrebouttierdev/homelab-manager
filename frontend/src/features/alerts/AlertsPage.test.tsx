import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  acknowledgeAlert,
  createAlertRule,
  deleteAlertRule,
  getAlertSummary,
  listAlertRules,
  listAlerts,
  updateAlertRule,
  type Alert,
  type AlertRule,
} from '../../lib/api/alerts'
import { listContainers } from '../../lib/api/monitoring'
import { RefreshProvider } from '../../lib/refresh'
import { AlertsPage } from './AlertsPage'

vi.mock('../../lib/api/alerts', () => ({
  listAlerts: vi.fn(),
  getAlertSummary: vi.fn(),
  acknowledgeAlert: vi.fn(),
  listAlertRules: vi.fn(),
  createAlertRule: vi.fn(),
  updateAlertRule: vi.fn(),
  deleteAlertRule: vi.fn(),
}))

vi.mock('../../lib/api/monitoring', () => ({ listContainers: vi.fn() }))

const session = vi.hoisted(() => ({ role: 'viewer' as 'viewer' | 'operator' | 'admin' }))
vi.mock('../../hooks/useSession', () => ({
  sessionQueryKey: ['session'],
  useSession: () => ({
    user: {
      id: 'u',
      username: 'u',
      displayName: 'u',
      role: session.role,
      mustChangePassword: false,
    },
    isPending: false,
    unauthenticated: false,
  }),
}))

const target = {
  kind: 'host' as const,
  metric: null,
  labels: {},
  containerId: null,
  containerName: null,
  unit: null,
  collector: null,
}

function alert(overrides: Partial<Alert>): Alert {
  return {
    id: 'a',
    ruleId: 'r',
    ruleName: 'Règle',
    ruleType: 'host_metric',
    severity: 'warning',
    state: 'firing',
    target,
    value: null,
    threshold: null,
    observed: null,
    firedAt: new Date(Date.now() - 10 * 60_000).toISOString(),
    acknowledgedAt: null,
    acknowledgedBy: null,
    resolvedAt: null,
    resolution: null,
    ...overrides,
  }
}

const openAlerts: Alert[] = [
  alert({
    id: 'disk',
    ruleName: 'Disque presque plein',
    target: { ...target, metric: 'filesystem.percent', labels: { mountpoint: '/' } },
    value: 91.5,
    threshold: 85,
  }),
  alert({
    id: 'unit',
    ruleName: 'Unité systemd en échec',
    ruleType: 'unit_failed',
    severity: 'critical',
    target: { ...target, kind: 'unit', unit: 'backup.service' },
    observed: 'failed (exit-code)',
  }),
]

const rules: AlertRule[] = [
  {
    id: 'default-disk',
    name: 'Disque presque plein',
    type: 'host_metric',
    severity: 'warning',
    enabled: true,
    default: true,
    params: { metric: 'filesystem.percent', operator: '>', threshold: 85, forSeconds: 300 },
    createdAt: '2026-11-24T00:00:00Z',
    updatedAt: '2026-11-24T00:00:00Z',
  },
  {
    id: 'custom',
    name: 'Web arrêté',
    type: 'container_state',
    severity: 'critical',
    enabled: true,
    default: false,
    params: { states: ['exited'], forSeconds: 60 },
    createdAt: '2026-11-24T00:00:00Z',
    updatedAt: '2026-11-24T00:00:00Z',
  },
]

const catalog = {
  hostMetrics: { 'filesystem.percent': ['mountpoint', 'device'], 'cpu.load1': [] },
  containerMetrics: { 'memory.percent': [], 'cpu.percent': [] },
  containerStates: ['running', 'exited'],
  collectors: ['system', 'docker', 'services'],
}

function renderPage() {
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <RefreshProvider>
        <MemoryRouter>
          <AlertsPage />
        </MemoryRouter>
      </RefreshProvider>
    </QueryClientProvider>,
  )
}

describe('AlertsPage', () => {
  beforeEach(() => {
    vi.mocked(getAlertSummary).mockResolvedValue({
      open: { info: 0, warning: 1, critical: 1 },
      resolvedLast24h: 3,
      meanResolutionSeconds24h: 840,
    })
    vi.mocked(listAlerts).mockImplementation((state) =>
      Promise.resolve(
        state === 'open'
          ? openAlerts
          : [
              alert({
                id: 'old',
                ruleName: 'Collecte Docker indisponible',
                state: 'resolved',
                resolvedAt: new Date().toISOString(),
                resolution: 'condition_cleared',
              }),
            ],
      ),
    )
    vi.mocked(listAlertRules).mockResolvedValue({ rules, catalog })
    vi.mocked(listContainers).mockResolvedValue([])
  })
  afterEach(() => vi.clearAllMocks())

  it('shows the indicators and the open alerts, most severe first, read-only for a viewer', async () => {
    session.role = 'viewer'
    const user = userEvent.setup()
    renderPage()

    expect(await screen.findByText('Unité systemd en échec')).toBeInTheDocument()
    expect(screen.getByText('14 min')).toBeInTheDocument()
    const rows = screen.getAllByRole('row').slice(1)
    expect(within(rows[0]).getByText('Critique')).toBeInTheDocument()
    expect(
      within(rows[0]).getByRole('link', { name: "Voir l'unité et son journal" }),
    ).toHaveAttribute('href', '/services?unit=backup.service')
    expect(within(rows[1]).getByText('91,5 % (seuil 85 %)')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Acquitter/ })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Historique' }))
    expect(await screen.findByText('Condition levée')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Règles' }))
    expect(
      await screen.findByText('Seuls les administrateurs modifient les règles.'),
    ).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Nouvelle règle' })).not.toBeInTheDocument()
  })

  it('lets an operator acknowledge a firing alert', async () => {
    session.role = 'operator'
    vi.mocked(acknowledgeAlert).mockResolvedValue(alert({ id: 'disk', state: 'acknowledged' }))
    const user = userEvent.setup()
    renderPage()

    const buttons = await screen.findAllByRole('button', { name: /Acquitter/ })
    await user.click(buttons[1])
    await waitFor(() => expect(acknowledgeAlert).toHaveBeenCalled())
    expect(vi.mocked(acknowledgeAlert).mock.calls[0][0]).toBe('disk')
  })

  it('lets an administrator create, toggle and delete rules', async () => {
    session.role = 'admin'
    vi.mocked(createAlertRule).mockResolvedValue(rules[1])
    vi.mocked(updateAlertRule).mockResolvedValue(rules[0])
    vi.mocked(deleteAlertRule).mockResolvedValue(undefined)
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('button', { name: 'Règles' }))

    expect(
      await screen.findByText('Espace disque utilisé (%) > 85 % pendant 5 min'),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Supprimer la règle Disque presque plein' }),
    ).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Nouvelle règle' }))
    await user.type(screen.getByLabelText('Nom'), 'Mémoire des conteneurs')
    await user.selectOptions(screen.getByLabelText('Type'), 'container_metric')
    expect(screen.getByLabelText('Métrique')).toHaveValue('memory.percent')
    await user.selectOptions(screen.getByLabelText('Métrique'), 'memory.percent')
    await user.clear(screen.getByLabelText('Seuil'))
    await user.type(screen.getByLabelText('Seuil'), '90,5')
    await user.click(screen.getByRole('button', { name: 'Enregistrer' }))
    await waitFor(() => expect(createAlertRule).toHaveBeenCalled())
    expect(vi.mocked(createAlertRule).mock.calls[0][0]).toEqual({
      name: 'Mémoire des conteneurs',
      type: 'container_metric',
      severity: 'warning',
      enabled: true,
      params: { metric: 'memory.percent', operator: '>', threshold: 90.5, forSeconds: 300 },
    })

    const disk = screen
      .getAllByRole('row')
      .find((row) => within(row).queryByText('Disque presque plein'))
    await user.click(within(disk as HTMLElement).getByRole('button', { name: 'Active' }))
    await waitFor(() => expect(updateAlertRule).toHaveBeenCalled())
    expect(vi.mocked(updateAlertRule).mock.calls[0]).toEqual([
      'default-disk',
      expect.objectContaining({ enabled: false, params: rules[0].params }),
    ])

    await user.click(screen.getByRole('button', { name: 'Supprimer la règle Web arrêté' }))
    await user.click(screen.getByRole('button', { name: 'Confirmer la suppression' }))
    await waitFor(() => expect(deleteAlertRule).toHaveBeenCalled())
    expect(vi.mocked(deleteAlertRule).mock.calls[0][0]).toBe('custom')
  })

  it('refuses an invalid rule before sending it', async () => {
    session.role = 'admin'
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('button', { name: 'Règles' }))
    await user.click(await screen.findByRole('button', { name: 'Nouvelle règle' }))
    await user.clear(screen.getByLabelText('Seuil'))
    await user.click(screen.getByRole('button', { name: 'Enregistrer' }))
    expect(await screen.findByText('Nom requis')).toBeInTheDocument()
    expect(screen.getByText('Seuil numérique requis')).toBeInTheDocument()
    expect(createAlertRule).not.toHaveBeenCalled()
  })
})
