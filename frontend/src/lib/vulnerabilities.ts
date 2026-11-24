import type { BadgeTone } from '../components/ui/Badge'
import type {
  Finding,
  FixState,
  ScanFailure,
  ScanTrigger,
  VulnerabilityScan,
  VulnerabilitySummary,
  VulnSeverity,
} from './api/vulnerabilities'
import { formatDateTime } from './format'

/** Severities, most severe first (API_CONTRACTS.md § 4.9). */
export const vulnSeverities: readonly VulnSeverity[] = [
  'critical',
  'high',
  'medium',
  'low',
  'negligible',
  'unknown',
]

const severityLabels: Record<VulnSeverity, string> = {
  critical: 'Critique',
  high: 'Élevée',
  medium: 'Moyenne',
  low: 'Faible',
  negligible: 'Négligeable',
  unknown: 'Inconnue',
}

const severityPlurals: Record<VulnSeverity, string> = {
  critical: 'Critiques',
  high: 'Élevées',
  medium: 'Moyennes',
  low: 'Faibles',
  negligible: 'Négligeables',
  unknown: 'Inconnues',
}

const severityTones: Record<VulnSeverity, BadgeTone> = {
  critical: 'danger',
  high: 'warning',
  medium: 'accent',
  low: 'info',
  negligible: 'neutral',
  unknown: 'neutral',
}

export function vulnSeverityLabel(severity: VulnSeverity): string {
  return severityLabels[severity] ?? severity
}

export function vulnSeverityTone(severity: VulnSeverity): BadgeTone {
  return severityTones[severity] ?? 'neutral'
}

const fixStates: Record<FixState, { label: string; tone: BadgeTone }> = {
  fixed: { label: 'Corrigeable', tone: 'success' },
  'not-fixed': { label: 'Non corrigée', tone: 'warning' },
  'wont-fix': { label: 'Ne sera pas corrigée', tone: 'neutral' },
  unknown: { label: 'Inconnu', tone: 'neutral' },
}

export function fixStateView(state: FixState): { label: string; tone: BadgeTone } {
  return fixStates[state] ?? fixStates.unknown
}

const failureLabels: Record<ScanFailure, string> = {
  unsupported_distro: 'distribution non prise en charge (Debian, Ubuntu et Alpine le sont)',
  inventory_failed: 'base de paquets illisible',
  vulnscan_not_installed: "Grype ou l'unité homelab-vulnscan.service n'est pas installé",
  vulnscan_not_allowed: "la règle polkit qui autorise l'analyse est absente",
  timeout: 'délai de 15 minutes dépassé',
  scan_failed:
    'Grype a échoué (base injoignable ou de plus de 5 jours ?), voir le journal de homelab-vulnscan',
  stale_result: "aucun résultat récent n'a été écrit",
  invalid_result: 'résultat illisible',
  interrupted: 'interrompue par un arrêt du Manager',
}

export function scanFailureLabel(failure: ScanFailure | null): string {
  return failure ? (failureLabels[failure] ?? failure) : 'cause inconnue'
}

const triggerLabels: Record<ScanTrigger, string> = {
  startup: 'au démarrage',
  schedule: 'planifiée',
  retry: 'nouvel essai',
  manual: 'manuelle',
}

export function scanTriggerLabel(trigger: ScanTrigger): string {
  return triggerLabels[trigger] ?? trigger
}

/** "24 h", "2 j", "1 j 12 h". */
export function formatScanInterval(hours: number): string {
  if (hours < 24) return `${hours} h`
  const days = Math.floor(hours / 24)
  const rest = hours % 24
  return rest > 0 ? `${days} j ${rest} h` : `${days} j`
}

export type ScanStatus = {
  /** "never" before any scan, then the state of the latest scan. */
  kind: 'never' | 'running' | 'succeeded' | 'failed'
  label: string
  tone: BadgeTone
  /** The failure of the latest scan, shown even when an older scan succeeded. */
  failure: string | null
  lastSuccess: string
  next: string
  database: string | null
  distro: string | null
}

/**
 * State of the scans for display: never an old result presented as recent
 * (SPEC_ADMINISTRATION.md § 8 ter.3), the failure of the latest scan stays
 * visible next to the date of the last success.
 */
export function scanStatus(summary: VulnerabilitySummary): ScanStatus {
  const latest = summary.latestScan
  const success = summary.lastSuccessfulScan
  const base = {
    lastSuccess: success?.finishedAt ? formatDateTime(success.finishedAt) : 'jamais',
    next: summary.running ? 'après celle en cours' : formatDateTime(summary.nextScanAt),
    database: success?.dbBuiltAt ? formatDateTime(success.dbBuiltAt) : null,
    distro: success?.distro ? `${success.distro.name} · ${success.packageCount} paquets` : null,
  }
  if (!latest)
    return {
      ...base,
      kind: 'never',
      label: 'Aucune analyse encore',
      tone: 'neutral',
      failure: null,
    }
  if (latest.state === 'running')
    return { ...base, kind: 'running', label: 'Analyse en cours', tone: 'accent', failure: null }
  if (latest.state === 'failed')
    return {
      ...base,
      kind: 'failed',
      label: 'Dernière analyse en échec',
      tone: 'danger',
      failure: `${formatDateTime(latest.finishedAt ?? latest.startedAt)} : ${scanFailureLabel(latest.failure)}`,
    }
  return { ...base, kind: 'succeeded', label: 'Analyse à jour', tone: 'success', failure: null }
}

export type TotalCard = { key: string; label: string; value: number; tone: BadgeTone }

/** Total then one card per severity, negligible and unknown included. */
export function totalCards(summary: Pick<VulnerabilitySummary, 'total' | 'totals'>): TotalCard[] {
  return [
    { key: 'total', label: 'Total', value: summary.total, tone: 'neutral' },
    ...vulnSeverities.map((severity) => ({
      key: severity,
      label: severityPlurals[severity],
      value: summary.totals[severity] ?? 0,
      tone: vulnSeverityTone(severity),
    })),
  ]
}

const advisorySources: ReadonlyArray<{ test: (url: URL) => boolean; label: string }> = [
  { test: (url) => url.hostname === 'nvd.nist.gov', label: 'NVD' },
  { test: (url) => url.hostname === 'security-tracker.debian.org', label: 'Avis Debian' },
  {
    test: (url) => url.hostname === 'ubuntu.com' || url.hostname.endsWith('.ubuntu.com'),
    label: 'Avis Ubuntu',
  },
  { test: (url) => url.hostname === 'access.redhat.com', label: 'Avis Red Hat' },
  { test: (url) => url.hostname === 'security.alpinelinux.org', label: 'Avis Alpine' },
  {
    test: (url) => url.hostname === 'github.com' && url.pathname.startsWith('/advisories/'),
    label: 'Avis GitHub',
  },
]

/**
 * The public page of a vulnerability, only from a recognised source
 * (SPEC_ADMINISTRATION.md § 8 ter.5): the data source first, then the URLs.
 */
export function publicAdvisory(finding: Pick<Finding, 'dataSource' | 'urls'>): {
  url: string
  label: string
} | null {
  for (const raw of [finding.dataSource, ...finding.urls]) {
    if (!raw) continue
    let url: URL
    try {
      url = new URL(raw)
    } catch {
      continue
    }
    if (url.protocol !== 'https:') continue
    const source = advisorySources.find((candidate) => candidate.test(url))
    if (source) return { url: url.toString(), label: source.label }
  }
  return null
}

export type FindingRow = {
  id: string
  vulnerabilityId: string
  severity: { label: string; tone: BadgeTone }
  package: string
  version: string
  fixedVersion: string
  fixState: { label: string; tone: BadgeTone }
  firstSeen: string
  resolved: string | null
}

export function toFindingRow(finding: Finding): FindingRow {
  return {
    id: finding.id,
    vulnerabilityId: finding.vulnerabilityId,
    severity: {
      label: vulnSeverityLabel(finding.severity),
      tone: vulnSeverityTone(finding.severity),
    },
    package: finding.package,
    version: finding.version,
    fixedVersion: finding.fixedVersions[0] ?? '—',
    fixState: fixStateView(finding.fixState),
    firstSeen: formatDateTime(finding.firstSeenAt),
    resolved: finding.resolvedAt ? formatDateTime(finding.resolvedAt) : null,
  }
}

export type FindingDetail = FindingRow & {
  description: string
  fixedVersions: string
  advisory: { url: string; label: string } | null
  references: string[]
  lastSeen: string
}

export function toFindingDetail(finding: Finding): FindingDetail {
  return {
    ...toFindingRow(finding),
    description: finding.description ?? 'Aucune description fournie par la source.',
    fixedVersions: finding.fixedVersions.length > 0 ? finding.fixedVersions.join(', ') : '—',
    advisory: publicAdvisory(finding),
    references: finding.urls,
    lastSeen: formatDateTime(finding.lastSeenAt),
  }
}

/** Changes of the latest successful scan: "3 nouvelles, 1 disparue". */
export function scanChanges(scan: VulnerabilityScan | null): string | null {
  if (!scan) return null
  const plural = (count: number, one: string, many: string) => `${count} ${count > 1 ? many : one}`
  return `${plural(scan.newCount, 'nouvelle', 'nouvelles')}, ${plural(scan.resolvedCount, 'disparue', 'disparues')} depuis l'analyse précédente`
}
