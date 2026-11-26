import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { getPowerSettings, requestPowerAction } from '../../lib/api/machine'
import { PowerCard } from './PowerCard'

vi.mock('../../lib/api/machine', () => ({
  getPowerSettings: vi.fn(),
  requestPowerAction: vi.fn(),
}))

vi.mock('../../hooks/useSession', () => ({
  useSession: () => ({ canManagePower: true }),
}))

function renderCard() {
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <PowerCard hostname="monoops" />
    </QueryClientProvider>,
  )
}

describe('PowerCard', () => {
  it('reboots only after the host name is typed', async () => {
    vi.mocked(getPowerSettings).mockResolvedValue({
      enabled: true,
      actions: ['reboot', 'poweroff'],
    })
    vi.mocked(requestPowerAction).mockResolvedValue({ requested: true, action: 'reboot' })
    const user = userEvent.setup()
    renderCard()

    await user.click(await screen.findByRole('button', { name: 'Redémarrer' }))
    const input = screen.getByLabelText('Saisissez « monoops » pour confirmer')
    await user.type(input, 'other')
    await user.click(screen.getByRole('button', { name: 'Confirmer : Redémarrer' }))
    expect(await screen.findByText('Saisissez exactement le nom de la machine')).toBeInTheDocument()
    expect(requestPowerAction).not.toHaveBeenCalled()

    await user.clear(input)
    await user.type(input, 'monoops')
    await user.click(screen.getByRole('button', { name: 'Confirmer : Redémarrer' }))
    await waitFor(() => expect(requestPowerAction).toHaveBeenCalledWith('reboot', 'monoops'))
    expect(await screen.findByRole('status')).toHaveTextContent('Redémarrage demandé')
  })

  it('explains how to enable the actions when they are disabled', async () => {
    vi.mocked(getPowerSettings).mockResolvedValue({
      enabled: false,
      actions: ['reboot', 'poweroff'],
    })
    renderCard()
    expect(await screen.findByText(/power_actions_enabled/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Éteindre' })).not.toBeInTheDocument()
  })
})
