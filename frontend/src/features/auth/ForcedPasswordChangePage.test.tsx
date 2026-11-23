import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { changePassword } from '../../lib/api/auth'
import { ForcedPasswordChangePage } from './ForcedPasswordChangePage'

const session = vi.hoisted(() => ({
  user: {
    id: 'a',
    username: 'alice',
    displayName: 'Alice',
    role: 'viewer' as const,
    mustChangePassword: true,
  },
  isPending: false,
  unauthenticated: false,
  error: null,
  mustChangePassword: true,
  roleLabel: 'Lecteur',
  canManageUsers: false,
  canManageSettings: false,
  canManagePower: false,
}))

vi.mock('../../hooks/useSession', () => ({
  sessionQueryKey: ['session'],
  useSession: () => session,
}))

vi.mock('../../lib/api/auth', () => ({
  changePassword: vi.fn().mockResolvedValue({ changed: true }),
  ApiError: class ApiError extends Error {
    status = 401
  },
}))

describe('ForcedPasswordChangePage', () => {
  it('asks the account to choose its own password', async () => {
    const user = userEvent.setup()
    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter>
          <ForcedPasswordChangePage />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(screen.getByText('Choisissez votre mot de passe')).toBeInTheDocument()
    await user.type(screen.getByLabelText('Mot de passe initial'), 'initial-password-1')
    await user.type(screen.getByLabelText('Nouveau mot de passe'), 'alice-own-password')
    await user.type(screen.getByLabelText('Confirmation'), 'alice-own-password')
    await user.click(screen.getByRole('button', { name: 'Enregistrer' }))

    await waitFor(() =>
      expect(changePassword).toHaveBeenCalledWith('initial-password-1', 'alice-own-password'),
    )
  })
})
