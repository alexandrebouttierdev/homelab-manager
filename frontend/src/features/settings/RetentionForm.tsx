import { Save } from 'lucide-react'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardFooter, CardHeader } from '../../components/ui/Card'
import type { MonitoringSettings } from '../../lib/api/monitoring'
import { useRetentionForm } from './useRetentionForm'

const numberFormat = new Intl.NumberFormat('fr-FR', { maximumFractionDigits: 1 })

export function RetentionForm({ settings }: { settings: MonitoringSettings }) {
  const {
    register,
    errors,
    isDirty,
    onSubmit,
    isPending,
    errorMessage,
    saved,
    estimate,
    customised,
  } = useRetentionForm(settings)

  return (
    <Card>
      <CardHeader
        title="Rétention des métriques"
        subtitle="Durée de conservation de l'historique et des événements"
        actions={
          <Badge tone={customised ? 'accent' : 'neutral'}>
            {customised ? 'Personnalisée' : 'Par défaut'}
          </Badge>
        }
      />
      <CardBody>
        <form className="space-y-3" onSubmit={onSubmit} noValidate>
          <div>
            <label
              htmlFor="metricsRetentionDays"
              className="mb-1.5 block font-mono text-[10px] uppercase tracking-wider text-text-secondary"
            >
              Conserver pendant (jours)
            </label>
            <input
              id="metricsRetentionDays"
              type="number"
              min={1}
              max={365}
              step={1}
              className="h-9 w-32 rounded-md border border-line bg-surface px-3 text-sm text-text-primary outline-none focus:border-accent"
              aria-invalid={Boolean(errors.metricsRetentionDays)}
              {...register('metricsRetentionDays', { valueAsNumber: true })}
            />
            {errors.metricsRetentionDays && (
              <p className="mt-1 text-xs text-danger">{errors.metricsRetentionDays.message}</p>
            )}
          </div>
          {estimate !== null && (
            <p className="text-xs text-text-secondary">
              Taille estimée de la base : ≈ {numberFormat.format(estimate)} Go avec une collecte
              toutes les {settings.systemCollectIntervalSeconds} s.
            </p>
          )}
          {errorMessage && (
            <div
              className="rounded-md border border-danger/30 bg-danger/10 px-3 py-2 text-xs text-danger"
              role="alert"
            >
              {errorMessage}
            </div>
          )}
          {saved && (
            <div
              className="rounded-md border border-success/30 bg-success/10 px-3 py-2 text-xs text-success"
              role="status"
            >
              Rétention enregistrée. Les données plus anciennes sont purgées immédiatement.
            </div>
          )}
          <Button
            type="submit"
            variant="primary"
            loading={isPending}
            disabled={!isDirty}
            icon={<Save className="h-4 w-4" />}
          >
            Enregistrer
          </Button>
        </form>
      </CardBody>
      <CardFooter className="font-mono text-[10px] text-text-muted">
        Valeur par défaut du service : {settings.metricsRetentionDefaultDays} jours. Réduire la
        rétention supprime définitivement l'historique plus ancien.
      </CardFooter>
    </Card>
  )
}
