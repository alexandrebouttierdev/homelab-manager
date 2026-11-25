import { BellOff, BellRing } from 'lucide-react'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/DataTable'
import { EmptyState } from '../../components/ui/EmptyState'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { SilenceForm } from './SilenceForm'
import { useSilencesPanel } from './useSilencesPanel'

export function SilencesPanel() {
  const page = useSilencesPanel()

  return (
    <div className="space-y-4">
      <p className="text-xs text-text-secondary">
        Un silence suspend les notifications des alertes qu’il couvre, de 15 minutes à 7 jours. Les
        alertes restent ouvertes et visibles. Posez un silence sur une alerte depuis l’onglet «
        Alertes ».
      </p>
      {page.canOperate && !page.silence.isOpen && (
        <Button size="sm" icon={<BellOff className="h-3.5 w-3.5" />} onClick={page.startGlobal}>
          Silence global
        </Button>
      )}
      {page.silence.isOpen && <SilenceForm model={page.silence} />}
      {page.liftError && (
        <p className="text-xs text-danger" role="alert">
          {page.liftError}
        </p>
      )}

      {page.state.isLoading && <LoadingState label="Chargement des silences…" rows={3} />}
      {page.state.isError && (
        <ErrorState message={page.state.error.message} onRetry={page.state.refetch} />
      )}
      {!page.state.isLoading && !page.state.isError && page.rows.length === 0 && (
        <EmptyState
          icon={BellRing}
          title="Aucun silence"
          description="Toutes les alertes sont notifiées aux canaux configurés."
        />
      )}
      {page.rows.length > 0 && (
        <Table>
          <THead>
            <TR>
              <TH>Portée</TH>
              <TH>Alertes couvertes</TH>
              <TH>Motif</TH>
              <TH>Période</TH>
              <TH>État</TH>
              <TH className="relative">
                <span className="sr-only">Actions</span>
              </TH>
            </TR>
          </THead>
          <TBody>
            {page.rows.map((row) => (
              <TR key={row.id}>
                <TD className="text-xs">{row.scope}</TD>
                <TD className="break-all text-text-primary">{row.subject}</TD>
                <TD>
                  <p className="text-text-primary">{row.reason}</p>
                  <p className="text-xs text-text-muted">par {row.createdBy}</p>
                </TD>
                <TD className="whitespace-nowrap text-xs">{row.period}</TD>
                <TD>
                  <Badge tone={row.status.tone}>{row.status.label}</Badge>
                </TD>
                <TD className="text-right">
                  {page.canOperate && row.liftable && (
                    <Button
                      size="sm"
                      loading={page.liftingId === row.id}
                      onClick={() => page.lift(row.id)}
                    >
                      Lever
                    </Button>
                  )}
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      )}
    </div>
  )
}
