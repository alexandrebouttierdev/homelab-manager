import { requestJSON } from './client'

/** Package vulnerabilities found by Grype (API_CONTRACTS.md § 4.9). */
export type VulnSeverity = 'critical' | 'high' | 'medium' | 'low' | 'negligible' | 'unknown'
export type FixState = 'fixed' | 'not-fixed' | 'wont-fix' | 'unknown'
export type ScanTrigger = 'startup' | 'schedule' | 'retry' | 'manual'
export type ScanState = 'running' | 'succeeded' | 'failed'
export type ScanFailure =
  | 'unsupported_distro'
  | 'inventory_failed'
  | 'vulnscan_not_installed'
  | 'vulnscan_not_allowed'
  | 'timeout'
  | 'scan_failed'
  | 'stale_result'
  | 'invalid_result'
  | 'interrupted'

export type SeverityTotals = Record<VulnSeverity, number>

export type VulnerabilityScan = {
  id: string
  trigger: ScanTrigger
  requestedBy: string | null
  state: ScanState
  failure: ScanFailure | null
  startedAt: string
  finishedAt: string | null
  grypeVersion: string | null
  dbBuiltAt: string | null
  distro: { id: string; version: string; name: string } | null
  packageCount: number
  totals: SeverityTotals
  newCount: number
  resolvedCount: number
}

export type Finding = {
  id: string
  vulnerabilityId: string
  severity: VulnSeverity
  package: string
  version: string
  packageType: string
  fixState: FixState
  fixedVersions: string[]
  description: string | null
  urls: string[]
  dataSource: string | null
  firstSeenAt: string
  lastSeenAt: string
  resolvedAt: string | null
}

export type VulnerabilitySummary = {
  totals: SeverityTotals
  total: number
  fixable: number
  latestScan: VulnerabilityScan | null
  lastSuccessfulScan: VulnerabilityScan | null
  running: boolean
  nextScanAt: string | null
  intervalHours: number
  defaultIntervalHours: number
  intervalSource: 'config' | 'user'
  recent: Finding[]
}

export type FindingQuery = {
  state: 'present' | 'resolved'
  severities: VulnSeverity[]
  fixableOnly: boolean
  search: string
  limit: number
  offset: number
}

export type FindingPage = { findings: Finding[]; total: number; limit: number; offset: number }

export function getVulnerabilitySummary(): Promise<VulnerabilitySummary> {
  return requestJSON<VulnerabilitySummary>('/api/v1/vulnerabilities/summary')
}

export function listFindings(query: FindingQuery): Promise<FindingPage> {
  const params = new URLSearchParams({
    state: query.state,
    limit: String(query.limit),
    offset: String(query.offset),
  })
  if (query.severities.length > 0) params.set('severity', query.severities.join(','))
  if (query.fixableOnly) params.set('fixable', 'true')
  if (query.search.trim()) params.set('q', query.search.trim())
  return requestJSON<FindingPage>(`/api/v1/vulnerabilities?${params.toString()}`)
}

export async function getFinding(id: string): Promise<Finding> {
  const body = await requestJSON<{ finding: Finding }>(
    `/api/v1/vulnerabilities/${encodeURIComponent(id)}`,
  )
  return body.finding
}

export async function requestScan(): Promise<VulnerabilityScan> {
  const body = await requestJSON<{ scan: VulnerabilityScan }>('/api/v1/vulnerabilities/scans', {
    method: 'POST',
  })
  return body.scan
}

export type ScanIntervalSettings = {
  intervalHours: number
  defaultIntervalHours: number
  intervalSource: 'config' | 'user'
}

export function updateScanInterval(intervalHours: number): Promise<ScanIntervalSettings> {
  return requestJSON<ScanIntervalSettings>('/api/v1/settings/vulnerability-scans', {
    method: 'PUT',
    body: JSON.stringify({ intervalHours }),
  })
}
