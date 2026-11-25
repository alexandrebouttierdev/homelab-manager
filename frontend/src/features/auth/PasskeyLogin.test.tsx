import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  ApiError,
  beginPasskeyLogin,
  completePasskeyLogin,
  passkeyLoginAvailable,
} from '../../lib/api/auth'
import { ThemeProvider } from '../../lib/theme'
import { getCredential } from '../../lib/webauthn'
import { LoginPage } from './LoginPage'

vi.mock('../../lib/api/auth', () => ({
  login: vi.fn(),
  passkeyLoginAvailable: vi.fn(),
  beginPasskeyLogin: vi.fn(),
  completePasskeyLogin: vi.fn(),
  ApiError: class ApiError extends Error {
    constructor(
      public status: number,
      message: string,
    ) {
      super(message)
    }
  },
}))

vi.mock('../../lib/webauthn', () => ({ getCredential: vi.fn(), webauthnSupported: () => true }))

function renderLogin() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <ThemeProvider>
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/login']}>
          <Routes>
            <Route path="/login" element={<LoginPage />} />
            <Route path="/" element={<p>Tableau de bord</p>} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>
    </ThemeProvider>,
  )
}

describe('login with a passkey alone', () => {
  beforeEach(() => vi.clearAllMocks())

  it('is not offered when the policy does not allow it', async () => {
    vi.mocked(passkeyLoginAvailable).mockResolvedValue(false)
    renderLogin()
    await screen.findByRole('button', { name: 'Se connecter' })
    await vi.waitFor(() => expect(passkeyLoginAvailable).toHaveBeenCalled())
    expect(screen.queryByRole('button', { name: /passkey/ })).not.toBeInTheDocument()
  })

  it('signs in with the passkey chosen in the browser', async () => {
    const options = { publicKey: { challenge: 'abc' } }
    const answer = { id: 'cred' }
    vi.mocked(passkeyLoginAvailable).mockResolvedValue(true)
    vi.mocked(beginPasskeyLogin).mockResolvedValue(options)
    vi.mocked(getCredential).mockResolvedValue(answer as never)
    vi.mocked(completePasskeyLogin).mockResolvedValue({
      user: { id: 'u1', username: 'alice', role: 'viewer' } as never,
    })
    renderLogin()

    await userEvent.click(
      await screen.findByRole('button', { name: 'Se connecter avec une passkey' }),
    )
    expect(await screen.findByText('Tableau de bord')).toBeInTheDocument()
    expect(getCredential).toHaveBeenCalledWith(options)
    expect(completePasskeyLogin).toHaveBeenCalledWith(answer)
  })

  it('shows why the passkey was refused', async () => {
    vi.mocked(passkeyLoginAvailable).mockResolvedValue(true)
    vi.mocked(beginPasskeyLogin).mockResolvedValue({ publicKey: {} })
    vi.mocked(getCredential).mockResolvedValue({} as never)
    vi.mocked(completePasskeyLogin).mockRejectedValue(
      new ApiError(401, 'Cette passkey ne permet pas de se connecter.'),
    )
    renderLogin()

    await userEvent.click(
      await screen.findByRole('button', { name: 'Se connecter avec une passkey' }),
    )
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Cette passkey ne permet pas de se connecter.',
    )
  })
})
