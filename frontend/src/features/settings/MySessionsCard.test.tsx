import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { listSessions, revokeOtherSessions, type SessionInfo } from '../../lib/api/security'
import { MySessionsCard } from './MySessionsCard'

vi.mock('../../lib/api/security', () => ({
  listSessions: vi.fn(),
  revokeSession: vi.fn(),
  revokeOtherSessions: vi.fn(),
}))

vi.mock('../../hooks/useSession', () => ({ sessionQueryKey: ['session'] }))

const current: SessionInfo = {
  id: 'current',
  userId: 'u1',
  username: 'alice',
  ipAddress: '192.0.2.4',
  userAgent: 'Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0',
  authMethod: 'password',
  createdAt: '2026-11-23T08:00:00Z',
  lastSeenAt: '2026-11-23T09:00:00Z',
  expiresAt: '2026-11-23T20:00:00Z',
  current: true,
}

describe('MySessionsCard', () => {
  it('marks the current session and revokes the others', async () => {
    vi.mocked(listSessions).mockResolvedValue([
      current,
      { ...current, id: 'other', current: false },
    ])
    vi.mocked(revokeOtherSessions).mockResolvedValue(1)
    const user = userEvent.setup()
    render(
      <QueryClientProvider
        client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
      >
        <MySessionsCard />
      </QueryClientProvider>,
    )

    expect(await screen.findByText('Cette session')).toBeInTheDocument()
    expect(listSessions).toHaveBeenCalledWith(false)
    await user.click(screen.getByRole('button', { name: 'Révoquer les autres sessions' }))
    await waitFor(() => expect(revokeOtherSessions).toHaveBeenCalled())
    expect(await screen.findByRole('status')).toHaveTextContent('1 session(s) révoquée(s).')
  })
})
