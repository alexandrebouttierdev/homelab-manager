import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import {
  getSecurityPolicy,
  listAudit,
  listSessions,
  revokeSession,
  updateSecurityPolicy,
} from '../../lib/api/security'
import {
  deleteAccount,
  listAccounts,
  resetAccountPassword,
  type Account,
} from '../../lib/api/users'
import { UsersPage } from './UsersPage'

vi.mock('../../lib/api/users', () => ({
  listAccounts: vi.fn(),
  createAccount: vi.fn(),
  updateAccount: vi.fn(),
  deleteAccount: vi.fn(),
  resetAccountPassword: vi.fn(),
}))

vi.mock('../../lib/api/security', () => ({
  listSessions: vi.fn(),
  revokeSession: vi.fn(),
  revokeOtherSessions: vi.fn(),
  getSecurityPolicy: vi.fn(),
  updateSecurityPolicy: vi.fn(),
  listAudit: vi.fn(),
  auditExportUrl: (_filter: unknown, format: string) => `/api/v1/audit/export?format=${format}`,
}))

vi.mock('../../hooks/useSession', () => ({
  sessionQueryKey: ['session'],
  useSession: () => ({
    user: {
      id: 'admin-id',
      username: 'admin',
      displayName: 'admin',
      role: 'admin',
      mustChangePassword: false,
    },
    isPending: false,
    unauthenticated: false,
    error: null,
    mustChangePassword: false,
    roleLabel: 'Administrateur',
    canManageUsers: true,
    canManageSettings: true,
    canManagePower: true,
  }),
}))

const admin: Account = {
  id: 'admin-id',
  username: 'admin',
  displayName: 'admin',
  role: 'admin',
  disabled: false,
  mustChangePassword: false,
  lastLoginAt: '2026-11-23T09:00:00Z',
  lastLoginIp: '192.0.2.1',
  createdAt: '2026-11-01T09:00:00Z',
}
const alice: Account = {
  ...admin,
  id: 'alice-id',
  username: 'alice',
  displayName: 'Alice',
  role: 'viewer',
  mustChangePassword: true,
  lastLoginAt: null,
  lastLoginIp: null,
}

function renderPage() {
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <MemoryRouter>
        <UsersPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('UsersPage', () => {
  it('lists the accounts and confirms before deleting one', async () => {
    vi.mocked(listAccounts).mockResolvedValue([admin, alice])
    vi.mocked(deleteAccount).mockResolvedValue({ deleted: true })
    const user = userEvent.setup()
    renderPage()

    expect(await screen.findByText('alice')).toBeInTheDocument()
    expect(screen.getByText('Mot de passe à changer')).toBeInTheDocument()
    expect(screen.getByText('(vous)')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Supprimer alice' }))
    expect(deleteAccount).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Confirmer la suppression' }))
    await waitFor(() => expect(deleteAccount).toHaveBeenCalledWith('alice-id'))
  })

  it('validates the new account before sending it', async () => {
    vi.mocked(listAccounts).mockResolvedValue([admin])
    const user = userEvent.setup()
    renderPage()

    await user.type(screen.getByLabelText('Identifiant'), 'Alice')
    await user.click(screen.getByRole('button', { name: 'Créer le compte' }))
    expect(
      await screen.findByText('Minuscules, chiffres, « . », « _ » ou « - », 64 au plus'),
    ).toBeInTheDocument()
    expect(screen.getByText('Au moins 12 caractères')).toBeInTheDocument()
  })

  it('lists the audit log with export links', async () => {
    vi.mocked(listAccounts).mockResolvedValue([admin])
    vi.mocked(listAudit).mockResolvedValue([
      {
        id: 'e1',
        actor: 'alice',
        targetType: 'route',
        targetId: 'GET /api/v1/users',
        action: 'access',
        outcome: 'denied',
        requestId: null,
        createdAt: '2026-11-23T09:00:00Z',
      },
    ])
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByRole('button', { name: "Journal d'audit" }))
    const table = await screen.findByRole('table')
    expect(within(table).getByText('Refusé')).toBeInTheDocument()
    expect(within(table).getByText('route · GET /api/v1/users')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Exporter en CSV' })).toHaveAttribute(
      'href',
      '/api/v1/audit/export?format=csv',
    )
  })

  it('validates the networks of the security policy', async () => {
    vi.mocked(listAccounts).mockResolvedValue([admin])
    vi.mocked(getSecurityPolicy).mockResolvedValue({
      idleTimeoutMinutes: 60,
      allowedNetworks: [],
      requireAdminSecondFactor: false,
      currentAddress: '192.0.2.1',
    })
    vi.mocked(updateSecurityPolicy).mockResolvedValue({
      idleTimeoutMinutes: 60,
      allowedNetworks: ['10.0.0.0/8'],
      requireAdminSecondFactor: false,
      currentAddress: '192.0.2.1',
    })
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByRole('button', { name: 'Politique' }))
    expect(await screen.findByText('192.0.2.1')).toBeInTheDocument()
    const networks = screen.getByLabelText(/Réseaux autorisés/)
    await user.type(networks, '10.0.0.1')
    await user.click(screen.getByRole('button', { name: 'Enregistrer' }))
    expect(
      await screen.findByText('Notation CIDR attendue, par exemple 192.168.1.0/24'),
    ).toBeInTheDocument()
    expect(updateSecurityPolicy).not.toHaveBeenCalled()

    await user.clear(networks)
    await user.type(networks, '10.0.0.0/8')
    await user.click(screen.getByRole('button', { name: 'Enregistrer' }))
    await waitFor(() =>
      expect(updateSecurityPolicy).toHaveBeenCalledWith({
        idleTimeoutMinutes: 60,
        allowedNetworks: ['10.0.0.0/8'],
        requireAdminSecondFactor: false,
      }),
    )
  })

  it('revokes a session of another account', async () => {
    vi.mocked(listAccounts).mockResolvedValue([admin])
    vi.mocked(revokeSession).mockResolvedValue({ revoked: true })
    vi.mocked(listSessions).mockResolvedValue([
      {
        id: 'bob-session',
        userId: 'bob-id',
        username: 'bob',
        ipAddress: '192.0.2.9',
        userAgent: 'curl/8.5.0',
        authMethod: 'password',
        createdAt: '2026-11-23T08:00:00Z',
        lastSeenAt: '2026-11-23T09:00:00Z',
        expiresAt: '2026-11-23T20:00:00Z',
        current: false,
      },
    ])
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByRole('button', { name: 'Sessions' }))
    await user.click(await screen.findByRole('button', { name: 'Révoquer la session curl de bob' }))
    await waitFor(() => expect(revokeSession).toHaveBeenCalledWith('bob-session'))
    expect(listSessions).toHaveBeenCalledWith(true)
  })

  it('resets the password of another account', async () => {
    vi.mocked(listAccounts).mockResolvedValue([admin, alice])
    vi.mocked(resetAccountPassword).mockResolvedValue({ ...alice, mustChangePassword: true })
    const user = userEvent.setup()
    renderPage()

    await screen.findByText('alice')
    expect(
      screen.queryByRole('button', { name: 'Réinitialiser le mot de passe de admin' }),
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Réinitialiser le mot de passe de alice' }))
    const input = screen.getByLabelText('Nouveau mot de passe initial')
    await user.type(input, 'short')
    await user.click(screen.getByRole('button', { name: 'Confirmer la réinitialisation' }))
    expect(await screen.findAllByText('Au moins 12 caractères')).not.toHaveLength(0)
    expect(resetAccountPassword).not.toHaveBeenCalled()

    await user.clear(input)
    await user.type(input, 'reset-password-1')
    await user.click(screen.getByRole('button', { name: 'Confirmer la réinitialisation' }))
    await waitFor(() =>
      expect(resetAccountPassword).toHaveBeenCalledWith('alice-id', 'reset-password-1'),
    )
    expect(await screen.findByText(/Mot de passe de alice réinitialisé/)).toBeInTheDocument()
  })
})
