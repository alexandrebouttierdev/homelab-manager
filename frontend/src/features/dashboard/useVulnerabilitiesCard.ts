import { useQuery } from '@tanstack/react-query'
import { loadState } from '../../hooks/loadState'
import { getVulnerabilitySummary } from '../../lib/api/vulnerabilities'
import { formatRelativeTime } from '../../lib/format'
import { useRefreshInterval } from '../../lib/refresh'
import { scanStatus, vulnSeverityLabel, vulnSeverityTone } from '../../lib/vulnerabilities'
import { vulnerabilitiesQueryKey } from '../host/useVulnerabilitiesPanel'

/** View model of the vulnerabilities section of the overview page. */
export function useVulnerabilitiesCard() {
  const refreshInterval = useRefreshInterval()
  const query = useQuery({
    queryKey: [...vulnerabilitiesQueryKey, 'summary'],
    queryFn: getVulnerabilitySummary,
    refetchInterval: refreshInterval || false,
  })
  const summary = query.data
  const status = summary ? scanStatus(summary) : null
  return {
    state: loadState(query),
    status,
    /** "never": no scan yet; "clean": a scan succeeded and found nothing. */
    empty: !summary
      ? null
      : !summary.lastSuccessfulScan
        ? 'never'
        : summary.total === 0
          ? 'clean'
          : null,
    counts: summary
      ? (['critical', 'high', 'medium', 'low'] as const).map((severity) => ({
          severity,
          label: vulnSeverityLabel(severity),
          tone: vulnSeverityTone(severity),
          value: summary.totals[severity],
        }))
      : [],
    total: summary?.total ?? 0,
    recent: (summary?.recent ?? []).map((finding) => ({
      id: finding.id,
      vulnerabilityId: finding.vulnerabilityId,
      package: finding.package,
      severity: {
        label: vulnSeverityLabel(finding.severity),
        tone: vulnSeverityTone(finding.severity),
      },
      found: formatRelativeTime(finding.firstSeenAt),
    })),
    link: '/host?tab=vulnerabilities',
  }
}
