import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  getVulnerabilitySummary,
  listFindings,
  requestScan,
  type Finding,
  type VulnerabilityScan,
  type VulnerabilitySummary,
} from '../../lib/api/vulnerabilities'
import { RefreshProvider } from '../../lib/refresh'
import { VulnerabilitiesCard } from '../dashboard/VulnerabilitiesCard'
import { VulnerabilitiesPanel } from './VulnerabilitiesPanel'

vi.mock('../../lib/api/vulnerabilities', () => ({
  getVulnerabilitySummary: vi.fn(),
  listFindings: vi.fn(),
  requestScan: vi.fn(),
  getFinding: vi.fn(),
  updateScanInterval: vi.fn(),
}))

const session = vi.hoisted(() => ({ role: 'viewer' as 'viewer' | 'operator' | 'admin' }))
vi.mock('../../hooks/useSession', () => ({
  useSession: () => ({
    user: {
      id: 'u',
      username: 'u',
      displayName: 'u',
      role: session.role,
      mustChangePassword: false,
    },
    canManageSettings: session.role === 'admin',
  }),
}))

const totals = { critical: 1, high: 1, medium: 0, low: 0, negligible: 0, unknown: 0 }

const success: VulnerabilityScan = {
  id: 'ok',
  trigger: 'schedule',
  requestedBy: null,
  state: 'succeeded',
  failure: null,
  startedAt: '2026-11-23T10:00:00Z',
  finishedAt: '2026-11-23T10:01:00Z',
  grypeVersion: '0.121.3',
  dbBuiltAt: '2026-11-23T04:00:00Z',
  distro: { id: 'debian', version: '13', name: 'Debian GNU/Linux 13 (trixie)' },
  packageCount: 412,
  totals,
  newCount: 2,
  resolvedCount: 0,
}

const findings: Finding[] = [
  {
    id: 'f1',
    vulnerabilityId: 'CVE-2026-1000',
    severity: 'critical',
    package: 'openssl',
    version: '3.5.1-1',
    packageType: 'deb',
    fixState: 'fixed',
    fixedVersions: ['3.5.1-2'],
    description: 'Dépassement de tampon.',
    urls: ['https://security-tracker.debian.org/tracker/CVE-2026-1000'],
    dataSource: null,
    firstSeenAt: '2026-11-23T10:01:00Z',
    lastSeenAt: '2026-11-23T10:01:00Z',
    resolvedAt: null,
  },
  {
    id: 'f2',
    vulnerabilityId: 'CVE-2026-2000',
    severity: 'high',
    package: 'curl',
    version: '8.14.1-2',
    packageType: 'deb',
    fixState: 'not-fixed',
    fixedVersions: [],
    description: null,
    urls: [],
    dataSource: null,
    firstSeenAt: '2026-11-23T10:01:00Z',
    lastSeenAt: '2026-11-23T10:01:00Z',
    resolvedAt: null,
  },
]

function summary(overrides: Partial<VulnerabilitySummary>): VulnerabilitySummary {
  return {
    totals,
    total: 2,
    fixable: 1,
    latestScan: success,
    lastSuccessfulScan: success,
    running: false,
    nextScanAt: '2026-11-24T10:01:00Z',
    intervalHours: 24,
    defaultIntervalHours: 24,
    intervalSource: 'config',
    recent: findings,
    ...overrides,
  }
}

function renderWith(node: React.ReactNode) {
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <RefreshProvider>
        <MemoryRouter>{node}</MemoryRouter>
      </RefreshProvider>
    </QueryClientProvider>,
  )
}

describe('VulnerabilitiesPanel', () => {
  beforeEach(() => {
    vi.mocked(getVulnerabilitySummary).mockResolvedValue(summary({}))
    vi.mocked(listFindings).mockResolvedValue({ findings, total: 120, limit: 50, offset: 0 })
  })
  afterEach(() => vi.clearAllMocks())

  it('shows the totals, the findings and the detail, read-only for a viewer', async () => {
    session.role = 'viewer'
    const user = userEvent.setup()
    renderWith(<VulnerabilitiesPanel />)
    expect(await screen.findByText('Analyse à jour')).toBeInTheDocument()
    expect(screen.getByText('Debian GNU/Linux 13 (trixie) · 412 paquets')).toBeInTheDocument()
    expect(await screen.findByText('CVE-2026-1000')).toBeInTheDocument()
    expect(screen.getByText('1–50 sur 120')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Analyser maintenant' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'CVE-2026-1000' }))
    const detail = screen.getByRole('region', { name: 'Détail de CVE-2026-1000' })
    expect(within(detail).getByText('Dépassement de tampon.')).toBeInTheDocument()
    expect(within(detail).getByRole('link', { name: /Avis Debian/ })).toHaveAttribute(
      'href',
      'https://security-tracker.debian.org/tracker/CVE-2026-1000',
    )

    await user.click(screen.getByRole('button', { name: 'Critique' }))
    await user.click(screen.getByRole('button', { name: 'Suivants' }))
    await user.type(screen.getByLabelText('Rechercher un identifiant ou un paquet'), 'openssl')
    await user.click(screen.getByRole('button', { name: 'Rechercher' }))
    await waitFor(() =>
      expect(vi.mocked(listFindings).mock.calls.at(-1)?.[0]).toEqual({
        state: 'present',
        severities: ['critical'],
        fixableOnly: false,
        search: 'openssl',
        limit: 50,
        offset: 0,
      }),
    )
    expect(vi.mocked(listFindings).mock.calls.some(([query]) => query.offset === 50)).toBe(true)
  })

  it('lets an operator start a scan and keeps a failure visible', async () => {
    session.role = 'operator'
    vi.mocked(getVulnerabilitySummary).mockResolvedValue(
      summary({
        latestScan: { ...success, id: 'ko', state: 'failed', failure: 'vulnscan_not_allowed' },
      }),
    )
    vi.mocked(requestScan).mockResolvedValue({ ...success, state: 'running' })
    const user = userEvent.setup()
    renderWith(<VulnerabilitiesPanel />)
    expect(await screen.findByText('Dernière analyse en échec')).toBeInTheDocument()
    expect(screen.getByRole('alert')).toHaveTextContent('règle polkit')
    await user.click(screen.getByRole('button', { name: 'Analyser maintenant' }))
    await waitFor(() => expect(requestScan).toHaveBeenCalled())
  })

  it('tells apart no scan yet and no vulnerability', async () => {
    vi.mocked(getVulnerabilitySummary).mockResolvedValue(
      summary({ latestScan: null, lastSuccessfulScan: null, total: 0, recent: [] }),
    )
    vi.mocked(listFindings).mockResolvedValue({ findings: [], total: 0, limit: 50, offset: 0 })
    renderWith(<VulnerabilitiesPanel />)
    expect(await screen.findAllByText('Aucune analyse encore')).not.toHaveLength(0)
  })
})

describe('VulnerabilitiesPanel remediation', () => {
  const openssl: Finding = { ...findings[0], fixedVersions: ['3.5.4-1'] }
  const command = 'sudo apt update && sudo apt install --only-upgrade openssl'

  function mockFindings(fixable: Finding[]) {
    vi.mocked(getVulnerabilitySummary).mockResolvedValue(summary({}))
    vi.mocked(listFindings).mockImplementation((query) =>
      Promise.resolve(
        query.fixableOnly && query.limit === 500
          ? { findings: fixable, total: fixable.length, limit: 500, offset: 0 }
          : { findings: [openssl, findings[1]], total: 2, limit: 50, offset: 0 },
      ),
    )
  }
  afterEach(() => vi.clearAllMocks())

  it('shows how to fix a fixed finding', async () => {
    mockFindings([openssl])
    const user = userEvent.setup()
    renderWith(<VulnerabilitiesPanel />)
    await user.click(await screen.findByRole('button', { name: 'CVE-2026-1000' }))
    const region = screen.getByRole('region', { name: 'Comment la corriger' })
    expect(within(region).getByText(command)).toBeInTheDocument()
    expect(within(region).getByText('Version visée : 3.5.4-1')).toBeInTheDocument()
  })

  it('shows no command when no fix is published', async () => {
    mockFindings([])
    const user = userEvent.setup()
    renderWith(<VulnerabilitiesPanel />)
    await user.click(await screen.findByRole('button', { name: 'CVE-2026-2000' }))
    const region = screen.getByRole('region', { name: 'Comment la corriger' })
    expect(within(region).getByText(/Aucune correction publiée/)).toBeInTheDocument()
    expect(region.querySelector('code')).toBeNull()
  })

  it('offers a fix-all command only when something is fixable', async () => {
    mockFindings([openssl])
    renderWith(<VulnerabilitiesPanel />)
    const card = await screen.findByRole('region', { name: 'Tout corriger' })
    expect(within(card).getByText(command)).toBeInTheDocument()
    expect(vi.mocked(listFindings).mock.calls.some(([q]) => q.fixableOnly && q.limit === 500)).toBe(
      true,
    )
  })

  it('hides the fix-all card without fixable findings', async () => {
    mockFindings([])
    renderWith(<VulnerabilitiesPanel />)
    await screen.findByText('CVE-2026-1000')
    expect(screen.queryByRole('region', { name: 'Tout corriger' })).not.toBeInTheDocument()
  })

  it('copies a command and confirms it', async () => {
    mockFindings([openssl])
    const user = userEvent.setup()
    const writeText = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined)
    renderWith(<VulnerabilitiesPanel />)
    const card = await screen.findByRole('region', { name: 'Tout corriger' })
    await user.click(within(card).getByRole('button', { name: 'Copier' }))
    expect(writeText).toHaveBeenCalledWith(command)
    expect(await screen.findByRole('status')).toHaveTextContent('Commande copiée')
  })

  it('has no copy button without a clipboard', async () => {
    mockFindings([openssl])
    userEvent.setup()
    Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true })
    renderWith(<VulnerabilitiesPanel />)
    await screen.findByRole('region', { name: 'Tout corriger' })
    expect(screen.queryByRole('button', { name: 'Copier' })).not.toBeInTheDocument()
  })
})

describe('VulnerabilitiesCard', () => {
  afterEach(() => vi.clearAllMocks())

  it('sums up the vulnerabilities on the dashboard', async () => {
    vi.mocked(getVulnerabilitySummary).mockResolvedValue(summary({}))
    renderWith(<VulnerabilitiesCard />)
    expect(await screen.findByText('CVE-2026-1000')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Voir les 2 vulnérabilités/ })).toHaveAttribute(
      'href',
      '/host?tab=vulnerabilities',
    )
  })

  it('says when a scan found nothing', async () => {
    vi.mocked(getVulnerabilitySummary).mockResolvedValue(
      summary({ total: 0, totals: { ...totals, critical: 0, high: 0 }, recent: [] }),
    )
    renderWith(<VulnerabilitiesCard />)
    expect(
      await screen.findByText('Aucune vulnérabilité connue dans les paquets.'),
    ).toBeInTheDocument()
  })
})
