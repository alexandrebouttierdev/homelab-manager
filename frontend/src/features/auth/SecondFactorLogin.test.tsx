import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { completeSecondFactor, login } from '../../lib/api/auth'
import { ThemeProvider } from '../../lib/theme'
import { LoginPage } from './LoginPage'

vi.mock('../../lib/api/auth', () => ({
  login: vi.fn(),
  completeSecondFactor: vi.fn(),
  needsSecondFactor: (response: object) => 'secondFactorRequired' in response,
  ApiError: class ApiError extends Error {
    status = 401
    code = ''
  },
}))

function renderLogin() {
  render(
    <ThemeProvider>
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter>
          <LoginPage />
        </MemoryRouter>
      </QueryClientProvider>
    </ThemeProvider>,
  )
}

describe('login with a second factor', () => {
  it('asks for the app code, then accepts a recovery code', async () => {
    vi.mocked(login).mockResolvedValue({
      secondFactorRequired: true,
      methods: ['totp', 'recovery'],
      expiresAt: '2026-11-23T10:05:00Z',
    })
    vi.mocked(completeSecondFactor).mockResolvedValue({
      user: {
        id: 'a',
        username: 'alice',
        displayName: 'Alice',
        role: 'viewer',
        mustChangePassword: false,
        mustEnrollSecondFactor: false,
      },
    })
    const user = userEvent.setup()
    renderLogin()

    await user.type(screen.getByLabelText("Nom d'utilisateur"), 'alice')
    await user.type(screen.getByLabelText('Mot de passe'), 'alice-own-password')
    await user.click(screen.getByRole('button', { name: 'Se connecter' }))

    const code = await screen.findByLabelText("Code de l'application")
    await user.type(code, '12345')
    await user.click(screen.getByRole('button', { name: 'Valider' }))
    expect(await screen.findByText('Code à 6 chiffres')).toBeInTheDocument()
    expect(completeSecondFactor).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'Utiliser un code de secours' }))
    await user.type(screen.getByLabelText('Code de secours'), 'k7pq-2mzx-b9ht')
    await user.click(screen.getByRole('button', { name: 'Valider' }))
    await waitFor(() =>
      expect(completeSecondFactor).toHaveBeenCalledWith('recovery', 'k7pq-2mzx-b9ht'),
    )
  })
})
