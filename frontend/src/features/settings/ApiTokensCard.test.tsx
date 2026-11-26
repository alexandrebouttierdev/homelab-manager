import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApiToken, listApiTokens, revokeApiToken, type ApiToken } from '../../lib/api/tokens'
import { ApiTokensCard } from './ApiTokensCard'

vi.mock('../../lib/api/tokens', () => ({
  listApiTokens: vi.fn(),
  createApiToken: vi.fn(),
  revokeApiToken: vi.fn(),
}))

const session = vi.hoisted(() => ({ role: 'viewer' as 'viewer' | 'operator' | 'admin' }))
vi.mock('../../hooks/useSession', () => ({
  sessionQueryKey: ['session'],
  useSession: () => ({
    user: { id: 'u', username: 'vera', displayName: 'Vera', role: session.role },
    isPending: false,
    unauthenticated: false,
  }),
}))

const existing: ApiToken = {
  id: 't1',
  name: 'Grafana',
  prefix: 'mop_abcdefgh',
  scopes: ['metrics:read', 'containers:read'],
  username: 'vera',
  createdAt: '2026-11-24T10:00:00Z',
  expiresAt: '2099-01-01T00:00:00Z',
  lastUsedAt: null,
}

function renderCard() {
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <ApiTokensCard />
    </QueryClientProvider>,
  )
}

describe('ApiTokensCard', () => {
  afterEach(() => {
    vi.clearAllMocks()
    session.role = 'viewer'
  })

  it('lists the tokens, creates one shown once and revokes one', async () => {
    vi.mocked(listApiTokens).mockResolvedValue({ tokens: [existing], scopes: [] })
    vi.mocked(createApiToken).mockResolvedValue({
      token: 'mop_secret-value',
      apiToken: { ...existing, id: 't2' },
    })
    vi.mocked(revokeApiToken).mockResolvedValue(undefined)
    const user = userEvent.setup()
    renderCard()

    expect(await screen.findByText('Grafana')).toBeInTheDocument()
    expect(screen.getByText('Métriques et synthèse, Conteneurs')).toBeInTheDocument()
    expect(screen.getByText('Jamais')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Nouveau jeton' }))
    await user.click(screen.getByRole('button', { name: 'Créer le jeton' }))
    expect(await screen.findByText('Nom requis')).toBeInTheDocument()
    expect(createApiToken).not.toHaveBeenCalled()
    await user.type(screen.getByLabelText('Nom'), 'Home Assistant')
    await user.click(screen.getByLabelText('Services système'))
    await user.click(screen.getByRole('button', { name: 'Créer le jeton' }))
    await waitFor(() =>
      expect(createApiToken).toHaveBeenCalledWith({
        name: 'Home Assistant',
        scopes: ['metrics:read', 'services:read'],
        expiresInDays: 90,
      }),
    )
    expect(await screen.findByText('mop_secret-value')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: "J'ai copié le jeton" }))
    expect(screen.queryByText('mop_secret-value')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Révoquer le jeton Grafana' }))
    await waitFor(() => expect(revokeApiToken).toHaveBeenCalledWith('t1'))
  })

  it('offers the action scopes to an operator only', async () => {
    vi.mocked(listApiTokens).mockResolvedValue({ tokens: [], scopes: [] })
    vi.mocked(createApiToken).mockResolvedValue({
      token: 'mop_secret-value',
      apiToken: { ...existing, id: 't3', scopes: ['containers:operate'] },
    })
    const user = userEvent.setup()
    renderCard()
    await user.click(await screen.findByRole('button', { name: 'Nouveau jeton' }))
    expect(screen.queryByLabelText('Agir sur les conteneurs')).not.toBeInTheDocument()
  })

  it('creates an action token for an operator', async () => {
    session.role = 'operator'
    vi.mocked(listApiTokens).mockResolvedValue({ tokens: [], scopes: [] })
    vi.mocked(createApiToken).mockResolvedValue({
      token: 'mop_secret-value',
      apiToken: { ...existing, id: 't3', scopes: ['containers:operate'] },
    })
    const user = userEvent.setup()
    renderCard()
    await user.click(await screen.findByRole('button', { name: 'Nouveau jeton' }))
    expect(screen.getByText(/inscrit au journal d'audit/)).toBeInTheDocument()
    await user.type(screen.getByLabelText('Nom'), 'Home Assistant')
    await user.click(screen.getByLabelText('Métriques et synthèse'))
    await user.click(screen.getByLabelText('Agir sur les conteneurs'))
    await user.click(screen.getByRole('button', { name: 'Créer le jeton' }))
    await waitFor(() =>
      expect(createApiToken).toHaveBeenCalledWith({
        name: 'Home Assistant',
        scopes: ['containers:operate'],
        expiresInDays: 90,
      }),
    )
  })
})
