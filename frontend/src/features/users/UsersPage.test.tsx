import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { deleteAccount, listAccounts, type Account } from '../../lib/api/users'
import { UsersPage } from './UsersPage'

vi.mock('../../lib/api/users', () => ({
  listAccounts: vi.fn(),
  createAccount: vi.fn(),
  updateAccount: vi.fn(),
  deleteAccount: vi.fn(),
}))

vi.mock('../../hooks/useSession', () => ({
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
})
