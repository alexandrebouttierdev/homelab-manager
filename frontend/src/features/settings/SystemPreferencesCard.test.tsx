import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '../../lib/api/client'
import { getSystemPreferences, updateSystemPreferences } from '../../lib/api/system'
import { SystemPreferencesCard } from './SystemPreferencesCard'

vi.mock('../../lib/api/system', () => ({
  getSystemPreferences: vi.fn(),
  updateSystemPreferences: vi.fn(),
}))

const session = vi.hoisted(() => ({ role: 'admin' as 'viewer' | 'operator' | 'admin' }))
vi.mock('../../hooks/useSession', () => ({
  sessionQueryKey: ['session'],
  useSession: () => ({
    user: { id: 'u', username: 'alice', displayName: 'Alice', role: session.role },
    isPending: false,
    unauthenticated: false,
  }),
}))

const current = { hostname: 'homelab', timezone: 'Europe/Paris', locale: 'fr_FR.UTF-8' }

function renderCard() {
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <SystemPreferencesCard />
    </QueryClientProvider>,
  )
}

describe('SystemPreferencesCard', () => {
  beforeEach(() => {
    session.role = 'admin'
    vi.mocked(getSystemPreferences).mockResolvedValue({
      enabled: true,
      localeEditable: true,
      preferences: current,
      timezones: ['Europe/Paris', 'UTC'],
    })
  })
  afterEach(() => vi.clearAllMocks())

  it('confirms the changes, then saves them', async () => {
    vi.mocked(updateSystemPreferences).mockResolvedValue({
      enabled: true,
      localeEditable: true,
      preferences: { ...current, hostname: 'nas' },
      timezones: [],
      changed: ['hostname'],
    })
    const user = userEvent.setup()
    renderCard()
    const hostname = await screen.findByLabelText('Nom de la machine')
    expect(hostname).toHaveValue('homelab')
    await user.clear(hostname)
    await user.type(hostname, 'nas')
    await user.click(screen.getByRole('button', { name: 'Enregistrer' }))
    expect(screen.getByRole('group', { name: 'Confirmer les changements' })).toHaveTextContent(
      'homelab → nas',
    )
    expect(updateSystemPreferences).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Confirmer les changements' }))
    await waitFor(() =>
      expect(updateSystemPreferences).toHaveBeenCalledWith({ ...current, hostname: 'nas' }),
    )
    expect(await screen.findByText('Modifié : Nom de la machine.')).toBeInTheDocument()
  })

  it('refuses an invalid host name before sending it', async () => {
    const user = userEvent.setup()
    renderCard()
    const hostname = await screen.findByLabelText('Nom de la machine')
    await user.clear(hostname)
    await user.type(hostname, '-nas')
    await user.click(screen.getByRole('button', { name: 'Enregistrer' }))
    expect(await screen.findByText(/sans tiret au début/)).toBeInTheDocument()
    expect(
      screen.queryByRole('group', { name: 'Confirmer les changements' }),
    ).not.toBeInTheDocument()
  })

  it('shows the refusal of the system', async () => {
    vi.mocked(updateSystemPreferences).mockRejectedValue(new ApiError(400, 'Fuseau refusé'))
    const user = userEvent.setup()
    renderCard()
    const zone = await screen.findByLabelText('Fuseau horaire')
    await user.clear(zone)
    await user.type(zone, 'Mars/Olympus')
    await user.click(screen.getByRole('button', { name: 'Enregistrer' }))
    await user.click(screen.getByRole('button', { name: 'Confirmer les changements' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Fuseau refusé')
  })

  it('is read-only for an operator and while disabled', async () => {
    session.role = 'operator'
    renderCard()
    expect(await screen.findByLabelText('Nom de la machine')).toHaveAttribute('readonly')
    expect(screen.queryByRole('button', { name: 'Enregistrer' })).not.toBeInTheDocument()
    expect(screen.getByText(/seuls les administrateurs/)).toBeInTheDocument()
  })

  it('tells an administrator the setting is off', async () => {
    vi.mocked(getSystemPreferences).mockResolvedValue({
      enabled: false,
      localeEditable: true,
      preferences: current,
      timezones: [],
    })
    renderCard()
    expect(await screen.findByText(/system_preferences_enabled/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Enregistrer' })).not.toBeInTheDocument()
  })

  it('keeps the language read-only where Debian forbids it', async () => {
    vi.mocked(getSystemPreferences).mockResolvedValue({
      enabled: true,
      localeEditable: false,
      preferences: current,
      timezones: [],
    })
    renderCard()
    expect(await screen.findByLabelText('Langue du système')).toHaveAttribute('readonly')
    expect(screen.getByLabelText('Nom de la machine')).not.toHaveAttribute('readonly')
    expect(screen.getByText(/dpkg-reconfigure locales/)).toBeInTheDocument()
  })
})
