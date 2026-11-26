import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  generateBackupSSHKey,
  getBackupConfig,
  getBackupRun,
  getBackupSummary,
  initBackupCopy,
  initBackupRepository,
  listBackupRuns,
  listBackupSnapshots,
  requestBackup,
  requestBackupOperation,
  setBackupCopy,
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
  requestBackupOperation: vi.fn(),
  connectBackupRepository: vi.fn(),
  getBackupRun: vi.fn(),
  setBackupCopy: vi.fn(),
  removeBackupCopy: vi.fn(),
  initBackupCopy: vi.fn(),
  connectBackupCopy: vi.fn(),
  generateBackupSSHKey: vi.fn(),
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
  details: null,
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
  lastCheck: null,
  lastCopy: null,
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
  sqliteDatabases: [],
  schedule: { enabled: true, frequency: 'daily', weekday: 0, time: '03:00' },
  retention: { enabled: false, keepDaily: 7, keepWeekly: 4, keepMonthly: 6, keepYearly: 0 },
  check: { enabled: true, subsetPercent: 5 },
  updatedAt: null,
  updatedBy: null,
  copy: null,
  sshKey: null,
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

  it('lets an administrator simulate a restore, then unlock the repository after confirmation', async () => {
    session.role = 'admin'
    vi.mocked(listBackupSnapshots).mockResolvedValue([
      {
        id: 'abcdef0123456789',
        shortId: 'abcdef01',
        time: '2026-11-24T03:00:02Z',
        hostname: 'monoops',
        paths: ['/etc'],
        tags: ['monoops'],
        totalFiles: 14,
        totalBytes: 65536,
        dataAdded: 2048,
      },
    ])
    vi.mocked(requestBackupOperation).mockResolvedValue({
      ...success,
      operation: 'restore',
      state: 'running',
      details: { dryRun: true, target: '/var/lib/monoops-backup/restore/r' },
    })
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('button', { name: 'Instantanés' }))
    await user.click(await screen.findByRole('button', { name: /Restaurer…/ }))
    await user.type(screen.getByLabelText(/Chemins à restaurer/), 'etc')
    expect(screen.getByText(/n'est pas un chemin absolu propre/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Simuler' })).toBeDisabled()
    await user.clear(screen.getByLabelText(/Chemins à restaurer/))
    await user.type(screen.getByLabelText(/Chemins à restaurer/), '/etc/hostname')
    await user.click(screen.getByRole('button', { name: 'Simuler' }))
    await waitFor(() =>
      expect(requestBackupOperation).toHaveBeenCalledWith('restore', {
        restore: { snapshotId: 'abcdef0123456789', include: ['/etc/hostname'], dryRun: true },
      }),
    )
    expect(await screen.findByText(/Simulation lancée/)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Configuration' }))
    await user.click(await screen.findByRole('button', { name: 'Forcer le déverrouillage' }))
    expect(screen.getByText(/supprime tous les verrous/)).toBeInTheDocument()
    expect(requestBackupOperation).toHaveBeenCalledTimes(1)
    await user.click(screen.getByRole('button', { name: 'Confirmer le déverrouillage' }))
    await waitFor(() => expect(requestBackupOperation).toHaveBeenLastCalledWith('unlock'))
  })

  it('restores in place only after a simulation of the same paths and an acknowledgement', async () => {
    session.role = 'admin'
    vi.mocked(listBackupSnapshots).mockResolvedValue([
      {
        id: 'abcdef0123456789',
        shortId: 'abcdef01',
        time: '2026-11-24T03:00:02Z',
        hostname: 'monoops',
        paths: ['/srv'],
        tags: ['monoops'],
        totalFiles: 14,
        totalBytes: 65536,
        dataAdded: 2048,
      },
    ])
    const simulation = {
      ...success,
      id: 'sim',
      operation: 'restore-in-place' as const,
      details: {
        dryRun: true,
        inPlace: true,
        target: '/',
        totalFiles: 4,
        filesRestored: 2,
        bytesRestored: 10,
      },
    }
    vi.mocked(requestBackupOperation).mockResolvedValue({ ...simulation, state: 'running' })
    vi.mocked(getBackupRun).mockResolvedValue(simulation)
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('button', { name: 'Instantanés' }))
    await user.click(await screen.findByRole('button', { name: /Restaurer…/ }))
    await user.click(screen.getByRole('button', { name: "À l'emplacement d'origine" }))
    const restore = screen.getByRole('button', { name: "Restaurer à l'emplacement d'origine" })
    expect(screen.getByText(/jamais tout l'instantané/)).toBeInTheDocument()
    await user.type(screen.getByLabelText(/Chemins à remettre en place/), '/etc/nginx')
    expect(screen.getByText(/passez par la quarantaine/)).toBeInTheDocument()
    await user.clear(screen.getByLabelText(/Chemins à remettre en place/))
    await user.type(screen.getByLabelText(/Chemins à remettre en place/), '/srv/app')
    expect(restore).toBeDisabled()
    await user.click(screen.getByRole('button', { name: 'Simuler' }))
    await waitFor(() =>
      expect(requestBackupOperation).toHaveBeenCalledWith('restore-in-place', {
        restore: { snapshotId: 'abcdef0123456789', include: ['/srv/app'], dryRun: true },
      }),
    )
    expect(await screen.findByText(/4 fichier\(s\) concerné\(s\)/)).toBeInTheDocument()
    expect(restore).toBeDisabled()
    await user.click(screen.getByLabelText(/Je remplace les fichiers actuels/))
    expect(restore).toBeEnabled()
    vi.mocked(requestBackupOperation).mockResolvedValue({
      ...simulation,
      id: 'real',
      state: 'running',
      details: { inPlace: true, target: '/' },
    })
    await user.click(restore)
    await waitFor(() =>
      expect(requestBackupOperation).toHaveBeenLastCalledWith('restore-in-place', {
        restore: { snapshotId: 'abcdef0123456789', include: ['/srv/app'], dryRun: false },
      }),
    )
    expect(
      await screen.findByText(/sauvegarde de sécurité des fichiers remplacés/),
    ).toBeInTheDocument()
  })

  it('prepares an archive of a path, then offers its download', async () => {
    session.role = 'admin'
    vi.mocked(listBackupSnapshots).mockResolvedValue([
      {
        id: 'abcdef0123456789',
        shortId: 'abcdef01',
        time: '2026-11-24T03:00:02Z',
        hostname: 'monoops',
        paths: ['/srv'],
        tags: ['monoops'],
        totalFiles: 14,
        totalBytes: 65536,
        dataAdded: 2048,
      },
    ])
    const finished = new Date().toISOString()
    vi.mocked(requestBackupOperation).mockResolvedValue({
      ...success,
      id: 'dump-run',
      operation: 'dump',
      state: 'running',
      details: { archivePath: '/srv/app' },
    })
    vi.mocked(getBackupRun).mockResolvedValue({
      ...success,
      id: 'dump-run',
      operation: 'dump',
      finishedAt: finished,
      details: { archivePath: '/srv/app', archiveBytes: 2048 },
    })
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('button', { name: 'Instantanés' }))
    await user.click(await screen.findByRole('button', { name: /Restaurer…/ }))
    await user.type(screen.getByLabelText(/Télécharger une archive/), '/srv/app')
    await user.click(screen.getByRole('button', { name: "Préparer l'archive" }))
    await waitFor(() =>
      expect(requestBackupOperation).toHaveBeenCalledWith('dump', {
        dump: { snapshotId: 'abcdef0123456789', path: '/srv/app' },
      }),
    )
    const link = await screen.findByRole('link', { name: 'Télécharger' })
    expect(link).toHaveAttribute('href', '/api/v1/backups/runs/dump-run/archive')
  })

  it('compares two snapshots, older first, and lists the changes', async () => {
    session.role = 'admin'
    const snapshot = (id: string, time: string) => ({
      id,
      shortId: id.slice(0, 8),
      time,
      hostname: 'monoops',
      paths: ['/etc'],
      tags: ['monoops'],
      totalFiles: 1,
      totalBytes: 1,
      dataAdded: 1,
    })
    vi.mocked(listBackupSnapshots).mockResolvedValue([
      snapshot('bbbbbbbb22222222', '2026-11-24T03:00:00Z'),
      snapshot('aaaaaaaa11111111', '2026-11-23T03:00:00Z'),
    ])
    const diffRun: BackupRun = {
      ...success,
      id: 'd1',
      operation: 'diff',
      origin: 'manual',
      stats: null,
      snapshotId: null,
      state: 'running',
      details: {
        diff: {
          from: 'aaaaaaaa11111111',
          to: 'bbbbbbbb22222222',
          changedFiles: 0,
          added: { files: 0, dirs: 0, others: 0, bytes: 0 },
          removed: { files: 0, dirs: 0, others: 0, bytes: 0 },
          changes: [],
          totalChanges: 0,
          truncated: false,
        },
      },
    }
    vi.mocked(requestBackupOperation).mockResolvedValue(diffRun)
    vi.mocked(getBackupRun).mockResolvedValue({
      ...diffRun,
      state: 'succeeded',
      details: {
        diff: {
          ...diffRun.details!.diff!,
          changedFiles: 1,
          added: { files: 1, dirs: 0, others: 0, bytes: 4 },
          changes: [
            { path: '/etc/hosts', modifier: 'M' },
            { path: '/etc/new.conf', modifier: '+' },
          ],
          totalChanges: 2,
        },
      },
    })
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('button', { name: 'Instantanés' }))
    const compare = await screen.findByRole('button', { name: 'Comparer' })
    expect(compare).toBeDisabled()
    await user.click(screen.getByLabelText('Comparer bbbbbbbb'))
    await user.click(screen.getByLabelText('Comparer aaaaaaaa'))
    await user.click(compare)
    await waitFor(() =>
      expect(requestBackupOperation).toHaveBeenCalledWith('diff', {
        diff: { from: 'aaaaaaaa11111111', to: 'bbbbbbbb22222222' },
      }),
    )
    expect(await screen.findByText('/etc/new.conf')).toBeInTheDocument()
    expect(screen.getByText(/1 modifié\(s\), 1 ajouté\(s\)/)).toBeInTheDocument()
    await user.selectOptions(screen.getByLabelText('Type de changement'), 'M')
    expect(screen.queryByText('/etc/new.conf')).not.toBeInTheDocument()
    expect(screen.getByText('/etc/hosts')).toBeInTheDocument()
  })

  it('lets an administrator set and create the copy repository', async () => {
    session.role = 'admin'
    const withCopy: BackupConfig = {
      ...config,
      copy: {
        repository: { type: 'local', location: '/media/usb/copy' },
        initialized: false,
        initializedAt: null,
        credentials: { password: false, username: false, secret: false },
        afterBackup: true,
      },
    }
    vi.mocked(setBackupCopy).mockResolvedValue(withCopy)
    vi.mocked(initBackupCopy).mockResolvedValue({
      password: 'copy-generated-password',
      run: { ...success, operation: 'copy', state: 'running' },
    })
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('button', { name: 'Configuration' }))
    const location = await screen.findByLabelText('Emplacement', { selector: '#copy-location' })
    await user.type(location, 'media/usb')
    await user.click(screen.getByRole('button', { name: 'Enregistrer le dépôt de copie' }))
    expect(await screen.findByText('Chemin absolu, sans « .. » ni « / » final')).toBeInTheDocument()
    expect(setBackupCopy).not.toHaveBeenCalled()
    await user.clear(location)
    await user.type(location, '/media/usb/copy')
    vi.mocked(getBackupConfig).mockResolvedValue(withCopy)
    await user.click(screen.getByRole('button', { name: 'Enregistrer le dépôt de copie' }))
    await waitFor(() => expect(setBackupCopy).toHaveBeenCalledTimes(1))
    expect(vi.mocked(setBackupCopy).mock.calls[0][0]).toEqual({
      repository: { type: 'local', location: '/media/usb/copy' },
      username: '',
      secret: '',
      afterBackup: true,
    })
    await user.click(await screen.findByRole('button', { name: 'Créer le dépôt de copie' }))
    expect(await screen.findByText('copy-generated-password')).toBeInTheDocument()
  })

  it('lets an administrator generate the SSH key, then save an SFTP repository', async () => {
    session.role = 'admin'
    const withKey: BackupConfig = {
      ...config,
      sshKey: {
        publicKey: 'ssh-ed25519 AAAAmanager monoops-backup',
        fingerprint: 'SHA256:manager',
        createdAt: '2026-11-25T08:00:00Z',
      },
    }
    vi.mocked(generateBackupSSHKey).mockResolvedValue(withKey)
    vi.mocked(updateBackupConfig).mockResolvedValue(withKey)
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('button', { name: 'Configuration' }))
    expect(screen.queryByText('Clé SSH du Manager')).not.toBeInTheDocument()

    await user.selectOptions(
      await screen.findByLabelText('Type', { selector: '#backup-type' }),
      'sftp',
    )
    expect(await screen.findByText('Clé SSH du Manager')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Générer la clé SSH' }))
    expect(
      await screen.findByDisplayValue('ssh-ed25519 AAAAmanager monoops-backup'),
    ).toBeInTheDocument()
    expect(screen.getByText(/Empreinte SHA256:manager/)).toBeInTheDocument()

    const location = screen.getByLabelText('Emplacement', { selector: '#backup-location' })
    await user.clear(location)
    await user.type(location, 'sftp://backup@nas.lan:2222/srv/restic')
    await user.click(screen.getByRole('button', { name: 'Enregistrer' }))
    expect(await screen.findByText("La clé d'hôte du serveur est requise")).toBeInTheDocument()
    expect(updateBackupConfig).not.toHaveBeenCalled()
    await user.type(screen.getByLabelText("Clé d'hôte du serveur"), 'nas.lan ssh-ed25519 AAAAhost')
    await user.click(screen.getByRole('button', { name: 'Enregistrer' }))
    await waitFor(() => expect(updateBackupConfig).toHaveBeenCalled())
    expect(vi.mocked(updateBackupConfig).mock.calls[0][0]).toMatchObject({
      repository: {
        type: 'sftp',
        location: 'sftp://backup@nas.lan:2222/srv/restic',
        hostKey: 'nas.lan ssh-ed25519 AAAAhost',
      },
      username: '',
      secret: '',
    })

    // Replacing the key asks first.
    await user.click(screen.getByRole('button', { name: 'Remplacer la clé' }))
    expect(await screen.findByText(/L'ancienne clé ne servira plus/)).toBeInTheDocument()
    expect(generateBackupSSHKey).toHaveBeenCalledTimes(1)
    await user.click(screen.getByRole('button', { name: 'Confirmer le remplacement' }))
    await waitFor(() => expect(generateBackupSSHKey).toHaveBeenCalledTimes(2))
  })

  it('hides the copy repository from a viewer when none is set', async () => {
    session.role = 'viewer'
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('button', { name: 'Configuration' }))
    await screen.findByText(/Seuls les administrateurs/)
    expect(screen.queryByText('Dépôt de copie')).not.toBeInTheDocument()
  })
})
