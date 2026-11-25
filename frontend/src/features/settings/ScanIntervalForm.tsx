import { Save, ShieldAlert } from 'lucide-react'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardFooter, CardHeader } from '../../components/ui/Card'
import { ErrorState } from '../../components/ui/ErrorState'
import { useScanIntervalForm } from './useScanIntervalForm'

export function ScanIntervalForm() {
  const form = useScanIntervalForm()
  return (
    <Card>
      <CardHeader
        title="Analyse des vulnérabilités"
        subtitle="Fréquence des analyses automatiques des paquets"
        icon={<ShieldAlert className="h-3.5 w-3.5" />}
        actions={
          <Badge tone={form.customised ? 'accent' : 'neutral'}>
            {form.customised ? 'Personnalisée' : 'Par défaut'}
          </Badge>
        }
      />
      <CardBody>
        {form.state.isError ? (
          <ErrorState message={form.state.error.message} onRetry={form.state.refetch} />
        ) : (
          <form className="space-y-3" onSubmit={form.onSubmit} noValidate>
            <fieldset
              disabled={form.readOnly || form.state.isLoading}
              className="min-w-0 space-y-3"
            >
              <div>
                <label
                  htmlFor="scanIntervalHours"
                  className="mb-1.5 block text-xs text-text-secondary"
                >
                  Analyser toutes les (heures)
                </label>
                <input
                  id="scanIntervalHours"
                  type="number"
                  min={6}
                  max={168}
                  step={1}
                  className="h-9 w-32 rounded-md border border-line bg-surface px-3 text-sm text-text-primary outline-none focus:border-accent"
                  aria-invalid={Boolean(form.errors.intervalHours)}
                  {...form.register('intervalHours', { valueAsNumber: true })}
                />
                {form.errors.intervalHours && (
                  <p className="mt-1 text-xs text-danger">{form.errors.intervalHours.message}</p>
                )}
              </div>
              {form.errorMessage && (
                <div
                  className="rounded-md border border-danger/30 bg-danger/10 px-3 py-2 text-xs text-danger"
                  role="alert"
                >
                  {form.errorMessage}
                </div>
              )}
              {form.saved && (
                <div
                  className="rounded-md border border-success/30 bg-success/10 px-3 py-2 text-xs text-success"
                  role="status"
                >
                  Intervalle enregistré : toutes les {form.current}.
                </div>
              )}
              <Button
                type="submit"
                variant="primary"
                loading={form.isPending}
                disabled={!form.isDirty}
                icon={<Save className="h-4 w-4" />}
              >
                Enregistrer
              </Button>
              {form.readOnly && (
                <p className="text-xs text-text-muted">Réservé aux administrateurs.</p>
              )}
            </fieldset>
          </form>
        )}
      </CardBody>
      <CardFooter className="text-xs text-text-muted">
        De 6 heures à 7 jours. Valeur par défaut du service : {form.defaultInterval ?? '—'}.
      </CardFooter>
    </Card>
  )
}
