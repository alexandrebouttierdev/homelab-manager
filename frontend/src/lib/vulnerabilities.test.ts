import { describe, expect, it } from 'vitest'
import type { Finding, VulnerabilityScan, VulnerabilitySummary } from './api/vulnerabilities'
import {
  formatScanInterval,
  publicAdvisory,
  scanChanges,
  scanFailureLabel,
  scanStatus,
  toFindingDetail,
  toFindingRow,
  totalCards,
} from './vulnerabilities'

const totals = { critical: 1, high: 2, medium: 3, low: 4, negligible: 5, unknown: 6 }

const scan = (overrides: Partial<VulnerabilityScan>): VulnerabilityScan => ({
  id: 's',
  trigger: 'schedule',
  requestedBy: null,
  state: 'succeeded',
  failure: null,
  startedAt: '2026-11-24T10:00:00Z',
  finishedAt: '2026-11-24T10:01:00Z',
  grypeVersion: '0.121.3',
  dbBuiltAt: '2026-11-24T04:00:00Z',
  distro: { id: 'debian', version: '13', name: 'Debian GNU/Linux 13 (trixie)' },
  packageCount: 412,
  totals,
  newCount: 3,
  resolvedCount: 1,
  ...overrides,
})

const summary = (overrides: Partial<VulnerabilitySummary>): VulnerabilitySummary => ({
  totals,
  total: 21,
  fixable: 4,
  latestScan: null,
  lastSuccessfulScan: null,
  running: false,
  nextScanAt: null,
  intervalHours: 24,
  defaultIntervalHours: 24,
  intervalSource: 'config',
  recent: [],
  ...overrides,
})

const finding: Finding = {
  id: 'f',
  vulnerabilityId: 'CVE-2026-1000',
  severity: 'critical',
  package: 'openssl',
  version: '3.5.1-1',
  packageType: 'deb',
  fixState: 'fixed',
  fixedVersions: ['3.5.1-2', '3.5.2-1'],
  description: null,
  urls: ['https://example.org/x', 'https://nvd.nist.gov/vuln/detail/CVE-2026-1000'],
  dataSource: 'http://security-tracker.debian.org/tracker/CVE-2026-1000',
  firstSeenAt: '2026-11-20T10:00:00Z',
  lastSeenAt: '2026-11-24T10:00:00Z',
  resolvedAt: null,
}

describe('vulnerabilities', () => {
  it('shows a failure of the latest scan next to the last success', () => {
    expect(scanStatus(summary({})).kind).toBe('never')
    expect(
      scanStatus(summary({ latestScan: scan({ state: 'running' }), running: true })).next,
    ).toBe('après celle en cours')
    const failed = scanStatus(
      summary({
        latestScan: scan({ state: 'failed', failure: 'vulnscan_not_allowed' }),
        lastSuccessfulScan: scan({}),
      }),
    )
    expect(failed.kind).toBe('failed')
    expect(failed.failure).toContain('règle polkit')
    expect(failed.lastSuccess).not.toBe('jamais')
    expect(failed.distro).toBe('Debian GNU/Linux 13 (trixie) · 412 paquets')
    expect(scanStatus(summary({ latestScan: scan({}), lastSuccessfulScan: scan({}) })).tone).toBe(
      'success',
    )
    expect(scanFailureLabel(null)).toBe('cause inconnue')
  })

  it('builds the totals and the intervals', () => {
    const cards = totalCards({ total: 21, totals })
    expect(cards.map((card) => card.value)).toEqual([21, 1, 2, 3, 4, 5, 6])
    expect(cards[1]).toMatchObject({ label: 'Critiques', tone: 'danger' })
    expect(formatScanInterval(6)).toBe('6 h')
    expect(formatScanInterval(48)).toBe('2 j')
    expect(formatScanInterval(36)).toBe('1 j 12 h')
    expect(scanChanges(scan({ newCount: 1, resolvedCount: 2 }))).toBe(
      "1 nouvelle, 2 disparues depuis l'analyse précédente",
    )
    expect(scanChanges(null)).toBeNull()
  })

  it('links only to a recognised public source over https', () => {
    expect(publicAdvisory(finding)).toEqual({
      url: 'https://nvd.nist.gov/vuln/detail/CVE-2026-1000',
      label: 'NVD',
    })
    expect(
      publicAdvisory({ dataSource: 'https://ubuntu.com/security/CVE-2026-1', urls: [] })?.label,
    ).toBe('Avis Ubuntu')
    expect(
      publicAdvisory({ dataSource: null, urls: ['https://github.com/advisories/GHSA-x'] })?.label,
    ).toBe('Avis GitHub')
    expect(
      publicAdvisory({ dataSource: 'https://github.com/evil/repo', urls: ['javascript:x'] }),
    ).toBeNull()
  })

  it('describes a finding', () => {
    const row = toFindingRow(finding)
    expect(row).toMatchObject({
      vulnerabilityId: 'CVE-2026-1000',
      fixedVersion: '3.5.1-2',
      severity: { label: 'Critique', tone: 'danger' },
      fixState: { label: 'Corrigeable', tone: 'success' },
      resolved: null,
    })
    const detail = toFindingDetail(finding)
    expect(detail.description).toBe('Aucune description fournie par la source.')
    expect(detail.fixedVersions).toBe('3.5.1-2, 3.5.2-1')
    expect(detail.advisory?.label).toBe('NVD')
    expect(toFindingRow({ ...finding, fixedVersions: [] }).fixedVersion).toBe('—')
  })
})
