import { Download } from 'lucide-react'
import { Badge } from '../../components/ui/Badge'
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/DataTable'
import { EmptyState } from '../../components/ui/EmptyState'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { Select } from '../../components/ui/Select'
import { useAuditPanel } from './useAuditPanel'

const labelClass = 'mb-1.5 block text-xs text-text-secondary'
const linkClass =
  'inline-flex h-9 items-center gap-2 rounded-md border border-line bg-card px-3.5 text-sm text-text-secondary hover:bg-elevated hover:text-text-primary'

export function AuditPanel() {
  const audit = useAuditPanel()

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-end gap-3">
        <div>
          <label htmlFor="auditActor" className={labelClass}>
            Acteur
          </label>
          <input
            id="auditActor"
            value={audit.actor}
            onChange={(event) => audit.setActor(event.target.value)}
            autoComplete="off"
            className="h-9 w-44 rounded-md border border-line bg-surface px-3 text-sm text-text-primary outline-none focus:border-accent"
          />
        </div>
        <div>
          <label htmlFor="auditOutcome" className={labelClass}>
            Résultat
          </label>
          <Select
            id="auditOutcome"
            value={audit.outcome}
            onChange={(event) => audit.setOutcome(event.target.value)}
          >
            {audit.outcomeOptions.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </Select>
        </div>
        <div>
          <label htmlFor="auditPeriod" className={labelClass}>
            Période
          </label>
          <Select
            id="auditPeriod"
            value={audit.period}
            onChange={(event) => audit.setPeriod(event.target.value)}
          >
            {audit.periodOptions.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </Select>
        </div>
        <div className="flex gap-2">
          <a href={audit.csvUrl} download className={linkClass}>
            <Download className="h-3.5 w-3.5" aria-hidden="true" />
            Exporter en CSV
          </a>
          <a href={audit.jsonUrl} download className={linkClass}>
            <Download className="h-3.5 w-3.5" aria-hidden="true" />
            Exporter en JSON
          </a>
        </div>
      </div>

      {audit.state.isError && (
        <ErrorState message={audit.state.error.message} onRetry={audit.state.refetch} />
      )}
      {audit.state.isLoading && <LoadingState label="Chargement du journal…" rows={3} />}
      {!audit.state.isLoading && !audit.state.isError && audit.rows.length === 0 && (
        <EmptyState title="Aucune entrée" description="Aucune entrée ne correspond aux filtres." />
      )}
      {audit.rows.length > 0 && (
        <Table minWidthClassName="min-w-[860px]">
          <THead>
            <TR>
              <TH>Date</TH>
              <TH>Acteur</TH>
              <TH>Action</TH>
              <TH>Cible</TH>
              <TH>Résultat</TH>
              <TH>Requête</TH>
            </TR>
          </THead>
          <TBody>
            {audit.rows.map((row) => (
              <TR key={row.id}>
                <TD className="whitespace-nowrap text-xs">{row.createdAt}</TD>
                <TD className="font-mono text-text-primary">{row.actor}</TD>
                <TD className="font-mono">{row.action}</TD>
                <TD className="max-w-[280px] truncate text-xs" title={row.target}>
                  {row.target}
                </TD>
                <TD>
                  <Badge tone={row.outcomeTone} dot>
                    {row.outcomeLabel}
                  </Badge>
                </TD>
                <TD className="text-xs text-text-muted">{row.requestId}</TD>
              </TR>
            ))}
          </TBody>
        </Table>
      )}
    </div>
  )
}
