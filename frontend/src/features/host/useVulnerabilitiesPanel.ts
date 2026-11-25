import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { loadState } from '../../hooks/loadState'
import { useSession } from '../../hooks/useSession'
import {
  getVulnerabilitySummary,
  listFindings,
  requestScan,
  type FindingQuery,
  type VulnSeverity,
} from '../../lib/api/vulnerabilities'
import { useRefreshInterval } from '../../lib/refresh'
import { canOperateAlerts } from '../../lib/roles'
import {
  fixAllCommands,
  formatScanInterval,
  scanChanges,
  scanStatus,
  toFindingDetail,
  toFindingRow,
  totalCards,
  vulnSeverities,
  vulnSeverityLabel,
} from '../../lib/vulnerabilities'

export const vulnerabilitiesQueryKey = ['vulnerabilities'] as const

const pageSize = 50
/** The fixable findings loaded to build the fix-all commands. */
const fixableQuery: FindingQuery = {
  state: 'present',
  severities: [],
  fixableOnly: true,
  search: '',
  limit: 500,
  offset: 0,
}
/** While a scan runs, its end is looked for every 5 s. */
const runningRefresh = 5000

/** Logic and view model of the vulnerabilities tab of the Machine page. */
export function useVulnerabilitiesPanel() {
  const refreshInterval = useRefreshInterval()
  const queryClient = useQueryClient()
  const { user } = useSession()

  const summaryQuery = useQuery({
    queryKey: [...vulnerabilitiesQueryKey, 'summary'],
    queryFn: getVulnerabilitySummary,
    refetchInterval: (query) =>
      query.state.data?.running ? runningRefresh : refreshInterval || false,
  })
  const running = summaryQuery.data?.running ?? false
  const lastSuccessID = summaryQuery.data?.lastSuccessfulScan?.id ?? null

  const [state, setState] = useState<FindingQuery['state']>('present')
  const [severities, setSeverities] = useState<VulnSeverity[]>([])
  const [fixableOnly, setFixableOnly] = useState(false)
  const [searchInput, setSearchInput] = useState('')
  const [search, setSearch] = useState('')
  const [offset, setOffset] = useState(0)
  const [selectedID, setSelectedID] = useState<string | null>(null)

  const query: FindingQuery = { state, severities, fixableOnly, search, limit: pageSize, offset }
  const findingsQuery = useQuery({
    // The latest successful scan is part of the key: a new result reloads
    // the list.
    queryKey: [...vulnerabilitiesQueryKey, 'findings', query, lastSuccessID],
    queryFn: () => listFindings(query),
  })
  const fixableQueryResult = useQuery({
    queryKey: [...vulnerabilitiesQueryKey, 'fixable', lastSuccessID],
    queryFn: () => listFindings(fixableQuery),
  })
  const fixablePage = fixableQueryResult.data
  const fixAll = useMemo(() => (fixablePage ? fixAllCommands(fixablePage) : []), [fixablePage])

  const [copied, setCopied] = useState<string | null>(null)
  const [copyError, setCopyError] = useState<string | null>(null)
  const clipboard = typeof navigator !== 'undefined' ? navigator.clipboard : undefined
  const copy = (text: string) => {
    if (!clipboard) return
    clipboard.writeText(text).then(
      () => {
        setCopyError(null)
        setCopied(text)
      },
      () => {
        setCopied(null)
        setCopyError('Copie impossible : sélectionner la commande à la main.')
      },
    )
  }
  const scan = useMutation({
    mutationFn: requestScan,
    onSettled: () => queryClient.invalidateQueries({ queryKey: vulnerabilitiesQueryKey }),
  })

  const findings = findingsQuery.data?.findings
  const rows = useMemo(() => (findings ?? []).map(toFindingRow), [findings])
  const selected = findings?.find((finding) => finding.id === selectedID)
  const total = findingsQuery.data?.total ?? 0
  const summary = summaryQuery.data
  // Every filter change starts again from the first page.
  const refilter =
    <T>(set: (value: T) => void) =>
    (value: T) => {
      set(value)
      setOffset(0)
      setSelectedID(null)
    }

  return {
    summaryState: loadState(summaryQuery),
    status: summary ? scanStatus(summary) : null,
    cards: summary ? totalCards(summary) : [],
    fixable: summary?.fixable ?? 0,
    changes: scanChanges(summary?.lastSuccessfulScan ?? null),
    interval: summary ? formatScanInterval(summary.intervalHours) : null,
    canScan: canOperateAlerts(user?.role),
    startScan: () => scan.mutate(),
    isStarting: scan.isPending,
    scanDisabled: running || scan.isPending,
    scanError: scan.error?.message ?? null,
    filters: {
      state,
      setState: refilter(setState),
      stateOptions: [
        { value: 'present' as const, label: 'Présentes' },
        { value: 'resolved' as const, label: 'Disparues' },
      ],
      severityOptions: vulnSeverities.map((value) => ({
        value,
        label: vulnSeverityLabel(value),
        pressed: severities.includes(value),
      })),
      toggleSeverity: (value: VulnSeverity) =>
        refilter(setSeverities)(
          severities.includes(value)
            ? severities.filter((candidate) => candidate !== value)
            : [...severities, value],
        ),
      fixableOnly,
      setFixableOnly: refilter(setFixableOnly),
      searchInput,
      setSearchInput,
      applySearch: () => refilter(setSearch)(searchInput.trim()),
    },
    findingsState: loadState(findingsQuery),
    rows,
    total,
    page: {
      from: total === 0 ? 0 : offset + 1,
      to: Math.min(offset + pageSize, total),
      hasPrevious: offset > 0,
      hasNext: offset + pageSize < total,
      previous: () => setOffset(Math.max(0, offset - pageSize)),
      next: () => setOffset(offset + pageSize),
    },
    selectedID,
    select: (id: string) => setSelectedID(id === selectedID ? null : id),
    fixAll: { state: loadState(fixableQueryResult), commands: fixAll },
    copy: { available: clipboard !== undefined, copy, copied, error: copyError },
    detail: selected ? toFindingDetail(selected) : null,
  }
}

export type VulnerabilitiesPanelModel = ReturnType<typeof useVulnerabilitiesPanel>
