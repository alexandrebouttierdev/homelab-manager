import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render } from '@testing-library/react'
import { screen } from '@testing-library/dom'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { ThemeProvider } from '../../lib/theme'
import { LoginPage } from './LoginPage'

vi.mock('../../lib/api/auth', () => ({
  login: vi.fn(),
  ApiError: class ApiError extends Error {
    status = 401
  },
}))

describe('LoginPage', () => {
  it('validates required fields before sending credentials', async () => {
    const client = new QueryClient()
    const user = userEvent.setup()
    render(
      <ThemeProvider>
        <QueryClientProvider client={client}>
          <MemoryRouter>
            <LoginPage />
          </MemoryRouter>
        </QueryClientProvider>
      </ThemeProvider>,
    )

    await user.click(screen.getByRole('button', { name: 'Se connecter' }))

    expect(await screen.findByText('Identifiant requis')).toBeInTheDocument()
    expect(screen.getByText('Au moins 8 caractères')).toBeInTheDocument()
  })

  it('renders the local supervision context', () => {
    const client = new QueryClient()
    render(
      <ThemeProvider>
        <QueryClientProvider client={client}>
          <MemoryRouter>
            <LoginPage />
          </MemoryRouter>
        </QueryClientProvider>
      </ThemeProvider>,
    )

    expect(screen.getByRole('heading', { name: 'Connexion' })).toBeInTheDocument()
    expect(screen.getByText('Homelab Manager · supervision locale')).toBeInTheDocument()
    expect(screen.getByText('Accès administrateur local')).toBeInTheDocument()
  })
})
