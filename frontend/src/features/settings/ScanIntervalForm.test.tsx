import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { getVulnerabilitySummary, updateScanInterval } from '../../lib/api/vulnerabilities'
import { ScanIntervalForm } from './ScanIntervalForm'

vi.mock('../../lib/api/vulnerabilities', () => ({
  getVulnerabilitySummary: vi.fn(),
  updateScanInterval: vi.fn(),
}))

const session = vi.hoisted(() => ({ admin: true }))
vi.mock('../../hooks/useSession', () => ({
  useSession: () => ({ canManageSettings: session.admin }),
}))

function renderForm() {
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <ScanIntervalForm />
    </QueryClientProvider>,
  )
}

describe('ScanIntervalForm', () => {
  afterEach(() => vi.clearAllMocks())

  it('lets an administrator change the interval within its bounds', async () => {
    session.admin = true
    vi.mocked(getVulnerabilitySummary).mockResolvedValue({
      intervalHours: 24,
      defaultIntervalHours: 24,
      intervalSource: 'config',
    } as Awaited<ReturnType<typeof getVulnerabilitySummary>>)
    vi.mocked(updateScanInterval).mockResolvedValue({
      intervalHours: 48,
      defaultIntervalHours: 24,
      intervalSource: 'user',
    })
    const user = userEvent.setup()
    renderForm()
    const input = await screen.findByLabelText('Analyser toutes les (heures)')
    await waitFor(() => expect(input).toHaveValue(24))
    await user.clear(input)
    await user.type(input, '2')
    await user.click(screen.getByRole('button', { name: 'Enregistrer' }))
    expect(await screen.findByText('Au moins 6 heures')).toBeInTheDocument()
    await user.clear(input)
    await user.type(input, '48')
    await user.click(screen.getByRole('button', { name: 'Enregistrer' }))
    await waitFor(() => expect(updateScanInterval).toHaveBeenCalledWith(48))
  })

  it('is read-only for other roles', async () => {
    session.admin = false
    vi.mocked(getVulnerabilitySummary).mockResolvedValue({
      intervalHours: 24,
      defaultIntervalHours: 24,
      intervalSource: 'config',
    } as Awaited<ReturnType<typeof getVulnerabilitySummary>>)
    renderForm()
    expect(await screen.findByText('Réservé aux administrateurs.')).toBeInTheDocument()
    expect(screen.getByLabelText('Analyser toutes les (heures)')).toBeDisabled()
  })
})
