import { describe, expect, it } from 'vitest'
import type { Finding, VulnerabilityScan, VulnerabilitySummary } from './api/vulnerabilities'
import {
  fixAllCommands,
  formatScanInterval,
  isKernelPackage,
  publicAdvisory,
  remediationFor,
  scanChanges,
  scanFailureLabel,
  scanStatus,
  toFindingDetail,
  toFindingRow,
  totalCards,
  upgradeCommand,
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

describe('remediation', () => {
  const debCommand = 'sudo apt update && sudo apt install --only-upgrade'
  const fixable = (name: string, packageType = 'deb'): Finding => ({
    ...finding,
    package: name,
    packageType,
  })

  it('builds an upgrade command, deduplicated and sorted', () => {
    expect(upgradeCommand('deb', ['openssl', 'libssl3', 'openssl'])).toBe(
      `${debCommand} libssl3 openssl`,
    )
    expect(upgradeCommand('apk', ['musl'])).toBe('sudo apk update && sudo apk upgrade musl')
    expect(upgradeCommand('rpm', ['openssl-libs'])).toBe('sudo dnf upgrade --refresh openssl-libs')
  })

  it('refuses unknown types, empty lists and unsafe names', () => {
    expect(upgradeCommand('deb', ['openssl; rm -rf /'])).toBeNull()
    expect(upgradeCommand('deb', ['$(id)'])).toBeNull()
    expect(upgradeCommand('binary', ['x'])).toBeNull()
    expect(upgradeCommand('deb', [])).toBeNull()
  })

  it('recognises kernel packages', () => {
    expect(isKernelPackage('deb', 'linux-image-6.12.0-amd64')).toBe(true)
    expect(isKernelPackage('deb', 'linux-libc-dev')).toBe(false)
    expect(isKernelPackage('rpm', 'kernel-core')).toBe(true)
    expect(isKernelPackage('rpm', 'kernel')).toBe(true)
    expect(isKernelPackage('apk', 'linux-lts')).toBe(true)
  })

  it('proposes the command and a follow-up for a fixable package', () => {
    const fixed = remediationFor({
      package: 'openssl',
      packageType: 'deb',
      fixState: 'fixed',
      fixedVersions: ['3.5.4-1'],
    })
    expect(fixed.kind).toBe('command')
    expect(fixed.command).toBe(upgradeCommand('deb', ['openssl']))
    expect(fixed.target).toBe('3.5.4-1')
    expect(fixed.followUp).toContain('services')
    const kernel = remediationFor({
      package: 'linux-image-amd64',
      packageType: 'deb',
      fixState: 'fixed',
      fixedVersions: ['6.12.1-1'],
    })
    expect(kernel.followUp).toContain('Redémarrer la machine')
  })

  it('stays generic for an unsupported package type', () => {
    const result = remediationFor({
      package: 'x',
      packageType: 'binary',
      fixState: 'fixed',
      fixedVersions: ['1'],
    })
    expect(result.kind).toBe('generic')
    expect(result.command).toBeNull()
  })

  it('gives no command when there is no fix', () => {
    for (const fixState of ['not-fixed', 'wont-fix', 'unknown'] as const) {
      const result = remediationFor({
        package: 'openssl',
        packageType: 'deb',
        fixState,
        fixedVersions: [],
      })
      expect(result.kind).toBe(fixState)
      expect(result.command).toBeNull()
    }
  })

  it('groups the fixable packages of a page by package type', () => {
    const all = fixAllCommands({
      findings: [fixable('openssl'), fixable('openssl'), fixable('curl')],
      total: 3,
    })
    expect(all).toHaveLength(1)
    expect(all[0]?.command).toBe(`${debCommand} curl openssl`)
    expect(all[0]?.packages).toBe(2)
    expect(all[0]?.complete).toBe(true)
  })

  it('falls back to a full upgrade when the page is partial', () => {
    const all = fixAllCommands({ findings: [fixable('openssl')], total: 620 })
    expect(all[0]?.complete).toBe(false)
    expect(all[0]?.command).toBe('sudo apt update && sudo apt upgrade')
    expect(all[0]?.note).toContain('620')
  })

  it('returns nothing without findings', () => {
    expect(fixAllCommands({ findings: [], total: 0 })).toEqual([])
  })

  it('adds the remediation to the finding detail', () => {
    expect(toFindingDetail(finding).remediation.kind).toBe('command')
  })
})
