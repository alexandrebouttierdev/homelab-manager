import { X } from 'lucide-react'
import { Badge } from '../../components/ui/Badge'
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/DataTable'
import { formatCount } from '../../lib/format'
import { inputClass } from '../alerts/FormField'
import type { BackupDiffModel } from './useBackupDiff'

/** Result of the comparison of two snapshots. */
export function BackupDiffPanel({ model }: { model: BackupDiffModel }) {
  const result = model.result
  if (!result) return null
  return (
    <div
      className="min-w-0 space-y-3 rounded-lg border border-line bg-card p-4"
      role="group"
      aria-label="Comparaison d'instantanés"
    >
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="text-sm font-semibold text-text-primary">
            Comparaison de <span className="font-mono">{result.from}</span> à{' '}
            <span className="font-mono">{result.to}</span>
          </p>
          <p className="mt-1 text-xs text-text-secondary">
            Ce qui a changé du plus ancien au plus récent, d&apos;après restic diff.
          </p>
        </div>
        <button
          type="button"
          aria-label="Fermer la comparaison"
          onClick={model.close}
          className="rounded-md p-1 text-text-muted hover:bg-elevated hover:text-text-primary"
        >
          <X className="h-4 w-4" />
        </button>
      </div>
      {result.running && (
        <p className="text-xs text-info" role="status">
          Comparaison en cours…
        </p>
      )}
      {result.failure && (
        <p className="text-xs text-danger" role="alert">
          {result.failure}
        </p>
      )}
      {result.summary && (
        <p className="text-xs text-text-primary" role="status">
          {result.summary}
        </p>
      )}
      {result.pathsHidden && (
        <p className="text-xs text-text-muted">
          Les chemins modifiés sont réservés aux administrateurs.
        </p>
      )}
      {result.summary && result.total > 0 && !result.pathsHidden && (
        <>
          <div className="flex flex-wrap items-center gap-2">
            <select
              aria-label="Type de changement"
              className={`${inputClass} w-auto`}
              value={model.filter}
              onChange={(event) => model.setFilter(event.target.value as typeof model.filter)}
            >
              {model.filterOptions.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
            <input
              type="search"
              aria-label="Filtrer les chemins"
              placeholder="Filtrer les chemins"
              className={`${inputClass} max-w-xs`}
              value={model.query}
              onChange={(event) => model.setQuery(event.target.value)}
            />
          </div>
          {result.truncated && (
            <p className="text-xs text-text-muted">
              {formatCount(result.total)} changements : seuls les 1 000 premiers sont listés.
            </p>
          )}
          <div className="max-h-[420px] overflow-auto">
            <Table minWidthClassName="min-w-[560px]">
              <THead>
                <tr>
                  <TH>Changement</TH>
                  <TH>Chemin</TH>
                </tr>
              </THead>
              <TBody>
                {result.rows.map((row) => (
                  <TR key={`${row.modifier}${row.path}`}>
                    <TD className="w-32">
                      <Badge tone={row.info.tone}>{row.info.label}</Badge>
                    </TD>
                    <TD className="break-all font-mono text-[11px]">{row.path}</TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          </div>
          {result.rows.length === 0 && (
            <p className="text-xs text-text-muted">Aucun chemin ne correspond au filtre.</p>
          )}
        </>
      )}
    </div>
  )
}
