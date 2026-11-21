import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
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
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <LoginPage />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Se connecter' }))

    expect(await screen.findByText('Identifiant requis')).toBeInTheDocument()
    expect(screen.getByText('Au moins 8 caractères')).toBeInTheDocument()
  })
})
