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
import {
  createChannel,
  createSilence,
  liftSilence,
  listChannels,
  listSilences,
  testChannel,
  updateChannel,
  type NotificationChannel,
  type Silence,
} from '../../lib/api/notifications'
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

vi.mock('../../lib/api/notifications', () => ({
  listSilences: vi.fn(),
  createSilence: vi.fn(),
  liftSilence: vi.fn(),
  listChannels: vi.fn(),
  createChannel: vi.fn(),
  updateChannel: vi.fn(),
  deleteChannel: vi.fn(),
  testChannel: vi.fn(),
}))

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
    targetKey: 'host:x',
    silenced: false,
    lastNotifiedAt: null,
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

const silences: Silence[] = [
  {
    id: 'silence-1',
    scope: 'rule',
    ruleId: 'r',
    ruleName: 'Disque presque plein',
    targetKey: null,
    reason: 'Migration du disque',
    createdBy: 'otto',
    startsAt: new Date(Date.now() - 60_000).toISOString(),
    endsAt: new Date(Date.now() + 3_600_000).toISOString(),
    liftedAt: null,
    liftedBy: null,
    active: true,
  },
]

const channels: NotificationChannel[] = [
  {
    id: 'channel-1',
    name: 'Téléphone',
    type: 'ntfy',
    enabled: true,
    minSeverity: 'warning',
    renotifyMinutes: 30,
    settings: { serverUrl: 'https://ntfy.sh', topic: 'homelab' },
    secrets: { accessToken: { defined: true, fingerprint: '3f9a0c1b7e42' } },
    lastSentAt: null,
    lastErrorAt: '2026-11-24T09:00:00Z',
    lastError: 'http_status',
    lastErrorStatus: 401,
    createdAt: '2026-11-24T08:00:00Z',
    updatedAt: '2026-11-24T08:00:00Z',
  },
]

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
    vi.mocked(listSilences).mockResolvedValue(silences)
    vi.mocked(listChannels).mockResolvedValue({ channels, secretsAvailable: true })
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

  it('lets an operator silence an alert and lift a silence, without the channels', async () => {
    session.role = 'operator'
    vi.mocked(listAlerts).mockImplementation((state) =>
      Promise.resolve(
        state === 'open' ? [openAlerts[0], { ...openAlerts[1], silenced: true }] : [],
      ),
    )
    vi.mocked(createSilence).mockResolvedValue(silences[0])
    vi.mocked(liftSilence).mockResolvedValue({ ...silences[0], active: false })
    const user = userEvent.setup()
    renderPage()

    expect(await screen.findByText('Sous silence')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Canaux' })).not.toBeInTheDocument()
    const silenceButtons = screen.getAllByRole('button', { name: 'Silence' })
    expect(silenceButtons).toHaveLength(1)
    await user.click(silenceButtons[0])
    await user.click(screen.getByRole('button', { name: 'Poser le silence' }))
    expect(await screen.findByText('Motif requis')).toBeInTheDocument()
    await user.selectOptions(screen.getByLabelText('Durée'), '240')
    await user.type(screen.getByLabelText('Motif'), 'Migration du disque')
    await user.click(screen.getByRole('button', { name: 'Poser le silence' }))
    await waitFor(() => expect(createSilence).toHaveBeenCalled())
    expect(vi.mocked(createSilence).mock.calls[0][0]).toEqual({
      scope: 'target',
      ruleId: 'r',
      targetKey: 'host:x',
      durationMinutes: 240,
      reason: 'Migration du disque',
    })

    await user.click(screen.getByRole('button', { name: 'Silences' }))
    expect(await screen.findByText('Règle « Disque presque plein »')).toBeInTheDocument()
    expect(screen.getByText(/^Encore /)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Lever' }))
    await waitFor(() => expect(liftSilence).toHaveBeenCalled())
    expect(vi.mocked(liftSilence).mock.calls[0][0]).toBe('silence-1')
  })

  it('lets an administrator test, create and edit channels without showing secrets', async () => {
    session.role = 'admin'
    vi.mocked(testChannel).mockResolvedValue(channels[0])
    vi.mocked(createChannel).mockResolvedValue(channels[0])
    vi.mocked(updateChannel).mockResolvedValue(channels[0])
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('button', { name: 'Canaux' }))

    expect(await screen.findByText('Téléphone')).toBeInTheDocument()
    expect(screen.getByText('défini · 3f9a0c1b7e42')).toBeInTheDocument()
    expect(screen.getByText(/Réponse HTTP 401 du serveur/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Tester' }))
    expect(await screen.findByText('Message de test envoyé.')).toBeInTheDocument()
    expect(vi.mocked(testChannel).mock.calls[0][0]).toBe('channel-1')

    await user.click(screen.getByRole('button', { name: 'Nouveau canal' }))
    await user.selectOptions(screen.getByLabelText('Type'), 'gotify')
    await user.type(screen.getByLabelText('Nom'), 'Gotify')
    await user.type(screen.getByLabelText('URL du serveur'), 'ftp://x')
    await user.click(screen.getByRole('button', { name: 'Enregistrer' }))
    expect(await screen.findByText('URL http ou https')).toBeInTheDocument()
    expect(screen.getByText("Jeton d'application requis")).toBeInTheDocument()
    expect(createChannel).not.toHaveBeenCalled()
    await user.clear(screen.getByLabelText('URL du serveur'))
    await user.type(screen.getByLabelText('URL du serveur'), 'http://192.168.1.10:8080')
    await user.type(screen.getByLabelText("Jeton d'application"), 'app-token')
    await user.click(screen.getByRole('button', { name: 'Enregistrer' }))
    await waitFor(() => expect(createChannel).toHaveBeenCalled())
    expect(vi.mocked(createChannel).mock.calls[0][0]).toEqual({
      name: 'Gotify',
      type: 'gotify',
      enabled: true,
      minSeverity: 'warning',
      renotifyMinutes: 30,
      settings: { serverUrl: 'http://192.168.1.10:8080' },
      secrets: { appToken: 'app-token' },
    })

    await user.click(await screen.findByRole('button', { name: 'Modifier le canal Téléphone' }))
    expect(screen.getByLabelText('Type')).toHaveValue('ntfy')
    expect(screen.getByLabelText("Jeton d'accès")).toHaveValue('')
    await user.click(screen.getByRole('button', { name: 'Enregistrer' }))
    await waitFor(() => expect(updateChannel).toHaveBeenCalled())
    expect(vi.mocked(updateChannel).mock.calls[0]).toEqual([
      'channel-1',
      expect.objectContaining({ type: 'ntfy', secrets: {} }),
    ])
  })
})
