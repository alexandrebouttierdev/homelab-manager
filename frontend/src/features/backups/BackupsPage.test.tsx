import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  getBackupConfig,
  getBackupSummary,
  initBackupRepository,
  listBackupRuns,
  listBackupSnapshots,
  requestBackup,
  updateBackupConfig,
  type BackupConfig,
  type BackupRun,
  type BackupSummary,
} from '../../lib/api/backups'
import { RefreshProvider } from '../../lib/refresh'
import { BackupsPage } from './BackupsPage'

vi.mock('../../lib/api/backups', () => ({
  getBackupSummary: vi.fn(),
  getBackupConfig: vi.fn(),
  updateBackupConfig: vi.fn(),
  initBackupRepository: vi.fn(),
  requestBackup: vi.fn(),
  listBackupRuns: vi.fn(),
  listBackupSnapshots: vi.fn(),
}))

const session = vi.hoisted(() => ({ role: 'viewer' as 'viewer' | 'operator' | 'admin' }))
vi.mock('../../hooks/useSession', () => ({
  sessionQueryKey: ['session'],
  useSession: () => ({
    user: { id: 'u', username: 'u', displayName: 'u', role: session.role },
    isPending: false,
    unauthenticated: false,
  }),
}))

const success: BackupRun = {
  id: 'r1',
  operation: 'backup',
  origin: 'schedule',
  requestedBy: null,
  state: 'succeeded',
  failure: null,
  message: null,
  warning: false,
  snapshotId: 'abcdef0123456789',
  stats: {
    filesNew: 3,
    filesChanged: 1,
    filesUnmodified: 10,
    dirsNew: 0,
    dirsChanged: 0,
    dirsUnmodified: 0,
    dataAdded: 2048,
    totalFilesProcessed: 14,
    totalBytesProcessed: 65536,
    durationSeconds: 2,
  },
  startedAt: '2026-11-24T03:00:00Z',
  finishedAt: '2026-11-24T03:00:02Z',
}

const failed: BackupRun = {
  ...success,
  id: 'r2',
  origin: 'manual',
  requestedBy: 'olivia',
  state: 'failed',
  failure: 'restic_failed',
  message: 'Fatal: wrong password',
  stats: null,
  snapshotId: null,
}

const summary: BackupSummary = {
  configured: true,
  initialized: true,
  running: null,
  lastRun: success,
  lastSuccess: success,
  nextRunAt: '2026-11-25T03:00:00Z',
}

const config: BackupConfig = {
  repository: { type: 'local', location: '/mnt/backup' },
  initialized: true,
  initializedAt: '2026-11-23T10:00:00Z',
  credentials: { password: true, username: false, secret: false },
  secretsAvailable: true,
  sources: ['/etc'],
  excludes: [],
  excludeCaches: true,
  excludeIfPresent: '',
  schedule: { enabled: true, frequency: 'daily', weekday: 0, time: '03:00' },
  updatedAt: null,
  updatedBy: null,
}

function renderPage() {
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <RefreshProvider>
        <MemoryRouter>
          <BackupsPage />
        </MemoryRouter>
      </RefreshProvider>
    </QueryClientProvider>,
  )
}

describe('BackupsPage', () => {
  beforeEach(() => {
    vi.mocked(getBackupSummary).mockResolvedValue(summary)
    vi.mocked(getBackupConfig).mockResolvedValue(config)
    vi.mocked(listBackupRuns).mockResolvedValue([failed, success])
    vi.mocked(listBackupSnapshots).mockResolvedValue([])
  })
  afterEach(() => vi.clearAllMocks())

  it('shows the state and the history, read-only for a viewer', async () => {
    session.role = 'viewer'
    const user = userEvent.setup()
    renderPage()
    expect(await screen.findByText('restic a échoué : Fatal: wrong password')).toBeInTheDocument()
    expect(screen.getByText('Sauvegarde planifiée')).toBeInTheDocument()
    expect(screen.getByText('instantané abcdef01')).toBeInTheDocument()
    expect(screen.getByText('Chaque jour à 03:00')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Sauvegarder maintenant/ })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Configuration' }))
    expect(
      await screen.findByText('Seuls les administrateurs modifient la configuration.'),
    ).toBeInTheDocument()
    expect(screen.getByLabelText('Emplacement')).toBeDisabled()
  })

  it('lets an operator start a backup', async () => {
    session.role = 'operator'
    vi.mocked(requestBackup).mockResolvedValue({ ...success, state: 'running' })
    const user = userEvent.setup()
    renderPage()
    const button = await screen.findByRole('button', { name: /Sauvegarder maintenant/ })
    await waitFor(() => expect(button).toBeEnabled())
    await user.click(button)
    await waitFor(() => expect(requestBackup).toHaveBeenCalled())
  })

  it('lets an administrator save the configuration and initialise the repository once', async () => {
    session.role = 'admin'
    const pending = { ...config, initialized: false, initializedAt: null }
    vi.mocked(getBackupSummary).mockResolvedValue({
      ...summary,
      initialized: false,
      lastRun: null,
      lastSuccess: null,
    })
    vi.mocked(getBackupConfig).mockResolvedValue(pending)
    vi.mocked(updateBackupConfig).mockResolvedValue({ ...pending, sources: ['/etc', '/srv'] })
    vi.mocked(initBackupRepository).mockResolvedValue({
      password: 'generated-password',
      run: success,
    })
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('button', { name: 'Configuration' }))

    const sources = await screen.findByLabelText('Chemins sauvegardés')
    await user.clear(sources)
    await user.type(sources, '/etc{enter}srv')
    await user.click(screen.getByRole('button', { name: 'Enregistrer' }))
    expect(await screen.findByText(/n'est pas un chemin absolu propre/)).toBeInTheDocument()
    expect(updateBackupConfig).not.toHaveBeenCalled()

    await user.clear(sources)
    await user.type(sources, '/etc{enter}/srv')
    await user.click(screen.getByRole('button', { name: 'Enregistrer' }))
    await waitFor(() => expect(updateBackupConfig).toHaveBeenCalled())
    expect(vi.mocked(updateBackupConfig).mock.calls[0][0]).toMatchObject({
      repository: { type: 'local', location: '/mnt/backup' },
      sources: ['/etc', '/srv'],
      username: '',
    })

    await user.click(await screen.findByRole('button', { name: 'Initialiser le dépôt' }))
    expect(await screen.findByText('generated-password')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: "J'ai conservé le mot de passe" }))
    expect(screen.queryByText('generated-password')).not.toBeInTheDocument()
  })
})
