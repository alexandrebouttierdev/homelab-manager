import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { beginTotp, confirmTotp, getSecondFactorStatus } from '../../lib/api/secondFactor'
import { SecondFactorCard } from './SecondFactorCard'
import { useSecondFactor } from './useSecondFactor'

vi.mock('../../lib/api/secondFactor', () => ({
  getSecondFactorStatus: vi.fn(),
  beginTotp: vi.fn(),
  confirmTotp: vi.fn(),
  regenerateRecoveryCodes: vi.fn(),
  disableSecondFactor: vi.fn(),
}))

vi.mock('../../hooks/useSession', () => ({ sessionQueryKey: ['session'] }))

function Harness() {
  return <SecondFactorCard model={useSecondFactor()} />
}

describe('SecondFactorCard', () => {
  it('enrolls an app with a QR code and shows the recovery codes once', async () => {
    vi.mocked(getSecondFactorStatus).mockResolvedValue({
      enabled: false,
      pending: false,
      recoveryCodesRemaining: 0,
      available: true,
    })
    vi.mocked(beginTotp).mockResolvedValue({
      secret: 'GEZDGNBVGY3TQOJQ',
      uri: 'otpauth://totp/x',
      qr: { size: 2, rows: ['10', '01'] },
    })
    const codes = ['aaaa-bbbb-cccc', 'dddd-eeee-ffff']
    vi.mocked(confirmTotp).mockResolvedValue(codes)
    const user = userEvent.setup()
    render(
      <QueryClientProvider
        client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
      >
        <Harness />
      </QueryClientProvider>,
    )

    await user.click(await screen.findByRole('button', { name: 'Activer' }))
    await user.type(screen.getByLabelText('Mot de passe actuel'), 'alice-own-password')
    await user.click(screen.getByRole('button', { name: 'Continuer' }))
    await waitFor(() => expect(beginTotp).toHaveBeenCalledWith('alice-own-password'))

    expect(
      await screen.findByRole('img', {
        name: "QR code à scanner avec l'application d'authentification",
      }),
    ).toBeInTheDocument()
    expect(screen.getByText('GEZD GNBV GY3T QOJQ')).toBeInTheDocument()
    await user.type(screen.getByLabelText("Code de l'application"), '123456')
    await user.click(screen.getByRole('button', { name: 'Vérifier et activer' }))
    await waitFor(() => expect(confirmTotp).toHaveBeenCalledWith('123456'))

    expect(await screen.findByText('aaaa-bbbb-cccc')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: "J'ai conservé mes codes" }))
    expect(screen.queryByText('aaaa-bbbb-cccc')).not.toBeInTheDocument()
  })

  it('explains that the secrets key is missing', async () => {
    vi.mocked(getSecondFactorStatus).mockResolvedValue({
      enabled: false,
      pending: false,
      recoveryCodesRemaining: 0,
      available: false,
    })
    render(
      <QueryClientProvider
        client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
      >
        <Harness />
      </QueryClientProvider>,
    )
    expect(await screen.findByText(/clé du coffre de secrets est absente/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Activer' })).toBeDisabled()
  })
})
