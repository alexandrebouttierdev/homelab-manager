import { useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { loadState } from '../../hooks/loadState'
import { auditExportUrl, listAudit, type AuditFilter } from '../../lib/api/security'
import {
  auditOutcomeOptions,
  auditPeriodOptions,
  auditPeriodStart,
  isAuditPeriod,
  toAuditRow,
  type AuditPeriod,
} from '../../lib/audit'

/** Logic and view model of the audit tab of UsersPage. */
export function useAuditPanel() {
  const [actor, setActor] = useState('')
  const [outcome, setOutcome] = useState('')
  const [period, setPeriod] = useState<AuditPeriod>('7d')

  const filter = useMemo<AuditFilter>(
    () => ({
      actor: actor.trim() || undefined,
      outcome: outcome || undefined,
      from: auditPeriodStart(period, new Date()),
    }),
    [actor, outcome, period],
  )
  const entries = useQuery({
    queryKey: ['audit', filter.actor ?? '', filter.outcome ?? '', period],
    queryFn: () => listAudit(filter),
  })
  const rows = useMemo(() => (entries.data ?? []).map(toAuditRow), [entries.data])

  return {
    state: loadState(entries),
    rows,
    actor,
    setActor,
    outcome,
    setOutcome,
    outcomeOptions: auditOutcomeOptions,
    period,
    setPeriod: (value: string) => {
      if (isAuditPeriod(value)) setPeriod(value)
    },
    periodOptions: auditPeriodOptions,
    csvUrl: auditExportUrl(filter, 'csv'),
    jsonUrl: auditExportUrl(filter, 'json'),
  }
}
