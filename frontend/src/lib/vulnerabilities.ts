import type { BadgeTone } from '../components/ui/Badge'
import type {
  Finding,
  FindingPage,
  FixState,
  ScanFailure,
  ScanTrigger,
  VulnerabilityScan,
  VulnerabilitySummary,
  VulnSeverity,
} from './api/vulnerabilities'
import { formatCount, formatDateTime } from './format'

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
  unsupported_distro:
    'distribution non prise en charge (Debian, Ubuntu, Alpine et les distributions RPM à base SQLite : Fedora, RHEL, Rocky Linux et AlmaLinux 9 et suivants)',
  inventory_failed: 'base de paquets illisible',
  vulnscan_not_installed: "Grype ou l'unité monoops-vulnscan.service n'est pas installé",
  vulnscan_not_allowed: "la règle polkit qui autorise l'analyse est absente",
  timeout: 'délai de 15 minutes dépassé',
  scan_failed:
    'Grype a échoué (base injoignable ou de plus de 5 jours ?), voir le journal de monoops-vulnscan',
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

type PackageManager = {
  label: string
  tool: string
  upgrade: (packages: string[]) => string
  fullUpgrade: string
  isKernel: (name: string) => boolean
}

const packageManagers: Record<string, PackageManager> = {
  deb: {
    label: 'Debian / Ubuntu (apt)',
    tool: 'apt',
    upgrade: (packages) =>
      `sudo apt update && sudo apt install --only-upgrade ${packages.join(' ')}`,
    fullUpgrade: 'sudo apt update && sudo apt upgrade',
    isKernel: (name) => name.startsWith('linux-image-'),
  },
  apk: {
    label: 'Alpine (apk)',
    tool: 'apk',
    upgrade: (packages) => `sudo apk update && sudo apk upgrade ${packages.join(' ')}`,
    fullUpgrade: 'sudo apk update && sudo apk upgrade',
    isKernel: (name) => name.startsWith('linux-'),
  },
  rpm: {
    label: 'Fedora / RHEL (dnf)',
    tool: 'dnf',
    upgrade: (packages) => `sudo dnf upgrade --refresh ${packages.join(' ')}`,
    fullUpgrade: 'sudo dnf upgrade --refresh',
    isKernel: (name) => name === 'kernel' || name.startsWith('kernel-'),
  },
}

/** Package names accepted in a command: nothing the shell could interpret. */
const packageNamePattern = /^[A-Za-z0-9][A-Za-z0-9+._-]{0,127}$/

function managerFor(packageType: string): PackageManager | null {
  return Object.hasOwn(packageManagers, packageType) ? (packageManagers[packageType] ?? null) : null
}

/** The command that upgrades these packages, or null if it cannot be built safely. */
export function upgradeCommand(packageType: string, packages: string[]): string | null {
  const manager = managerFor(packageType)
  if (!manager || packages.length === 0) return null
  if (!packages.every((name) => packageNamePattern.test(name))) return null
  return manager.upgrade([...new Set(packages)].sort())
}

export function fullUpgradeCommand(packageType: string): string | null {
  return managerFor(packageType)?.fullUpgrade ?? null
}

export function isKernelPackage(packageType: string, name: string): boolean {
  return managerFor(packageType)?.isKernel(name) ?? false
}

export type Remediation = {
  kind: 'command' | 'generic' | 'not-fixed' | 'wont-fix' | 'unknown'
  summary: string
  command: string | null
  target: string | null
  followUp: string | null
}

const genericSummary = 'Mettre à jour les paquets du système, puis relancer une analyse.'

const verify = 'puis relancer une analyse pour vérifier.'
const regularFollowUp = `Relancer les services qui utilisent ce paquet (needrestart sur Debian), ou redémarrer la machine, ${verify}`
const kernelFollowUps: Record<string, string> = {
  deb: `Redémarrer la machine pour charger le nouveau noyau, retirer les anciens noyaux (sudo apt autoremove sur Debian/Ubuntu, sudo dnf remove --oldinstallonly sur Fedora/RHEL), ${verify}`,
  rpm: `Redémarrer la machine pour charger le nouveau noyau, retirer les anciens noyaux (sudo apt autoremove sur Debian/Ubuntu, sudo dnf remove --oldinstallonly sur Fedora/RHEL), ${verify}`,
  apk: `Redémarrer la machine pour charger le nouveau noyau, ${verify}`,
}
const partialFollowUp = `Redémarrer la machine si le noyau a été mis à jour, sinon relancer les services concernés, ${verify}`

function followUpFor(packageType: string, kernel: boolean): string {
  return kernel ? (kernelFollowUps[packageType] ?? regularFollowUp) : regularFollowUp
}

/**
 * Debian kernel packages carry their version in their name: a fix is a new
 * package pulled by the metapackage, so only a full upgrade installs it.
 */
function isVersionedDebKernel(packageType: string, name: string): boolean {
  return packageType === 'deb' && /^linux-image-\d/.test(name)
}

/** How to fix a finding; the Manager never runs it, the user does. */
export function remediationFor(
  finding: Pick<Finding, 'package' | 'packageType' | 'fixState' | 'fixedVersions'>,
): Remediation {
  const none = { command: null, target: null, followUp: null }
  switch (finding.fixState) {
    case 'not-fixed':
      return {
        ...none,
        kind: 'not-fixed',
        summary: 'Aucune correction publiée pour l’instant : relancer une analyse plus tard.',
      }
    case 'wont-fix':
      return {
        ...none,
        kind: 'wont-fix',
        summary: 'La distribution ne prévoit pas de correction pour cette version.',
      }
    case 'unknown':
      return { ...none, kind: 'unknown', summary: genericSummary }
    case 'fixed': {
      const command = isVersionedDebKernel(finding.packageType, finding.package)
        ? fullUpgradeCommand(finding.packageType)
        : upgradeCommand(finding.packageType, [finding.package])
      if (!command) return { ...none, kind: 'generic', summary: genericSummary }
      return {
        kind: 'command',
        summary: 'Mettre à jour le paquet devrait corriger cette vulnérabilité.',
        command,
        target: finding.fixedVersions[0] ?? null,
        followUp: followUpFor(
          finding.packageType,
          isKernelPackage(finding.packageType, finding.package),
        ),
      }
    }
  }
}

export type FixAllCommand = {
  packageType: string
  /** Identifies the command for the copy feedback. */
  slot: string
  label: string
  copyLabel: string
  command: string
  /** "3 paquets", or null when the page is partial. */
  packagesLabel: string | null
  complete: boolean
  note: string | null
  followUp: string
}

/**
 * One command per package manager to fix the fixable findings of a page. When
 * the page does not hold every finding, a full upgrade replaces the list.
 */
export function fixAllCommands(page: Pick<FindingPage, 'findings' | 'total'>): FixAllCommand[] {
  const complete = page.total <= page.findings.length
  const byType = new Map<string, Set<string>>()
  for (const finding of page.findings) {
    if (finding.fixState !== 'fixed' || !managerFor(finding.packageType)) continue
    const names = byType.get(finding.packageType) ?? new Set<string>()
    names.add(finding.package)
    byType.set(finding.packageType, names)
  }
  const result: FixAllCommand[] = []
  for (const [packageType, names] of byType) {
    const manager = managerFor(packageType)
    if (!manager) continue
    const list = [...names]
    const hasKernel = list.some((name) => isKernelPackage(packageType, name))
    let command: string | null
    let note: string | null = null
    if (!complete) {
      command = fullUpgradeCommand(packageType)
      note = `Plus de 500 vulnérabilités corrigeables (${page.total}) : mise à jour complète du système.`
    } else if (list.some((name) => isVersionedDebKernel(packageType, name))) {
      command = fullUpgradeCommand(packageType)
      note =
        'Le noyau Debian change de nom à chaque version : il est mis à jour par la mise à jour complète.'
    } else {
      command = upgradeCommand(packageType, list)
      if (!command) {
        command = fullUpgradeCommand(packageType)
        note =
          'Un nom de paquet inattendu empêche une commande ciblée : mise à jour complète du système.'
      }
    }
    if (!command) continue
    result.push({
      packageType,
      slot: `fixAll:${packageType}`,
      label: manager.label,
      copyLabel: `Copier la commande ${manager.tool}`,
      command,
      packagesLabel: complete
        ? `${formatCount(names.size)} paquet${names.size > 1 ? 's' : ''}`
        : null,
      complete,
      note,
      followUp: complete ? followUpFor(packageType, hasKernel) : partialFollowUp,
    })
  }
  return result
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
  remediation: Remediation
  slot: string
  copyLabel: string
}

export function toFindingDetail(finding: Finding): FindingDetail {
  return {
    ...toFindingRow(finding),
    description: finding.description ?? 'Aucune description fournie par la source.',
    fixedVersions: finding.fixedVersions.length > 0 ? finding.fixedVersions.join(', ') : '—',
    advisory: publicAdvisory(finding),
    references: finding.urls,
    lastSeen: formatDateTime(finding.lastSeenAt),
    remediation: remediationFor(finding),
    slot: `detail:${finding.id}`,
    copyLabel: `Copier la commande de ${finding.vulnerabilityId}`,
  }
}

/** Changes of the latest successful scan: "3 nouvelles, 1 disparue". */
export function scanChanges(scan: VulnerabilityScan | null): string | null {
  if (!scan) return null
  const plural = (count: number, one: string, many: string) => `${count} ${count > 1 ? many : one}`
  return `${plural(scan.newCount, 'nouvelle', 'nouvelles')}, ${plural(scan.resolvedCount, 'disparue', 'disparues')} depuis l'analyse précédente`
}
