import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { getDockerInventory } from '../../lib/api/docker'
import { listContainers } from '../../lib/api/monitoring'
import { RefreshProvider } from '../../lib/refresh'
import { ImagesPage } from './ImagesPage'

vi.mock('../../lib/api/monitoring', () => ({ listContainers: vi.fn() }))
vi.mock('../../lib/api/docker', () => ({ getDockerInventory: vi.fn() }))

describe('ImagesPage', () => {
  it('lists the images that no container uses', async () => {
    vi.mocked(listContainers).mockResolvedValue([])
    vi.mocked(getDockerInventory).mockResolvedValue({
      images: [
        {
          id: 'sha256:aaa',
          tags: ['redis:7'],
          createdAt: '2026-11-18T10:00:00Z',
          sizeBytes: 1048576,
          containers: 0,
        },
        {
          id: 'sha256:bbb',
          tags: ['nginx:stable'],
          createdAt: '2026-11-18T10:00:00Z',
          sizeBytes: 1048576,
          containers: 1,
        },
      ],
      volumes: [],
      networks: [],
      truncated: false,
      observedAt: '2026-11-25T09:00:00Z',
      status: 'ok',
      lastSuccessAt: '2026-11-25T09:00:00Z',
    })
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={client}>
        <RefreshProvider>
          <MemoryRouter>
            <ImagesPage />
          </MemoryRouter>
        </RefreshProvider>
      </QueryClientProvider>,
    )

    const card = await screen.findByRole('region', { name: 'Images inutilisées' })
    expect(await within(card).findByText('redis:7')).toBeInTheDocument()
    expect(within(card).queryByText('nginx:stable')).not.toBeInTheDocument()
    expect(within(card).getByText(/1 élément\(s\) · 1 Mo/)).toBeInTheDocument()
  })

  it('says why the list is missing when the proxy refuses it', async () => {
    vi.mocked(listContainers).mockResolvedValue([])
    vi.mocked(getDockerInventory).mockResolvedValue({
      images: [],
      volumes: [],
      networks: [],
      truncated: false,
      observedAt: null,
      status: 'unavailable',
      lastSuccessAt: null,
    })
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={client}>
        <RefreshProvider>
          <MemoryRouter>
            <ImagesPage />
          </MemoryRouter>
        </RefreshProvider>
      </QueryClientProvider>,
    )
    expect(await screen.findByText(/proxy Docker n’autorise pas encore/)).toBeInTheDocument()
  })
})
