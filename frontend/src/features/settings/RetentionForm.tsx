import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Save } from 'lucide-react'
import { useForm, useWatch } from 'react-hook-form'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardFooter, CardHeader } from '../../components/ui/Card'
import { updateRetention, type MonitoringSettings } from '../../lib/api/monitoring'
import {
  estimatedStorageGigabytes,
  retentionSchema,
  type RetentionValues,
} from '../../schemas/retention'

const numberFormat = new Intl.NumberFormat('fr-FR', { maximumFractionDigits: 1 })

export function RetentionForm({ settings }: { settings: MonitoringSettings }) {
  const queryClient = useQueryClient()
  const {
    register,
    handleSubmit,
    control,
    formState: { errors, isDirty },
  } = useForm<RetentionValues>({
    resolver: zodResolver(retentionSchema),
    values: { metricsRetentionDays: settings.metricsRetentionDays },
  })
  const mutation = useMutation({
    mutationFn: (values: RetentionValues) => updateRetention(values.metricsRetentionDays),
    onSuccess: (updated) => queryClient.setQueryData(['settings'], updated),
  })

  const days = useWatch({ control, name: 'metricsRetentionDays' })
  const estimate = Number.isFinite(days)
    ? estimatedStorageGigabytes(days, settings.systemCollectIntervalSeconds)
    : null

  return (
    <Card>
      <CardHeader
        title="Rétention des métriques"
        subtitle="Durée de conservation de l'historique et des événements"
        actions={
          <Badge tone={settings.metricsRetentionSource === 'user' ? 'accent' : 'neutral'}>
            {settings.metricsRetentionSource === 'user' ? 'Personnalisée' : 'Par défaut'}
          </Badge>
        }
      />
      <CardBody>
        <form
          className="space-y-3"
          onSubmit={handleSubmit((values) => mutation.mutate(values))}
          noValidate
        >
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
          {estimate !== null && !errors.metricsRetentionDays && (
            <p className="text-xs text-text-secondary">
              Taille estimée de la base : ≈ {numberFormat.format(estimate)} Go avec une collecte
              toutes les {settings.systemCollectIntervalSeconds} s.
            </p>
          )}
          {mutation.isError && (
            <div
              className="rounded-md border border-danger/30 bg-danger/10 px-3 py-2 text-xs text-danger"
              role="alert"
            >
              {mutation.error.message}
            </div>
          )}
          {mutation.isSuccess && !isDirty && (
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
            loading={mutation.isPending}
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
