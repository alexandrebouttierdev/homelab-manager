import { Power } from 'lucide-react'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { usePowerActions } from './usePowerActions'

export function PowerCard({ hostname }: { hostname: string }) {
  const power = usePowerActions(hostname)
  if (!power.visible) return null

  return (
    <Card>
      <CardHeader
        title="Alimentation"
        subtitle="Redémarrer ou éteindre cette machine"
        icon={<Power className="h-3.5 w-3.5" />}
      />
      <CardBody className="space-y-3">
        {power.state.isError && (
          <p className="text-xs text-danger" role="alert">
            {power.state.error.message}
          </p>
        )}
        {!power.state.isLoading && !power.state.isError && !power.enabled && (
          <p className="text-xs text-text-secondary">
            Désactivé dans la configuration du Manager (<code>power_actions_enabled</code>) ;
            l'activation exige aussi la règle polkit fournie. Voir le README.
          </p>
        )}
        {power.enabled && !power.selected && (
          <div className="flex flex-wrap gap-2">
            {power.actions.map((item) => (
              <Button
                key={item.action}
                variant="danger"
                size="sm"
                onClick={() => power.choose(item.action)}
              >
                {item.label}
              </Button>
            ))}
          </div>
        )}
        {power.selected && !power.accepted && (
          <form className="space-y-3" onSubmit={power.onConfirm} noValidate>
            <p className="text-xs text-text-secondary">{power.selected.warning}</p>
            <div>
              <label
                htmlFor="confirmHostname"
                className="mb-1.5 block font-mono text-[10px] uppercase tracking-wider text-text-secondary"
              >
                Saisissez « {power.hostname} » pour confirmer
              </label>
              <input
                id="confirmHostname"
                autoComplete="off"
                className="h-9 w-full rounded-md border border-line bg-surface px-3 font-mono text-sm text-text-primary outline-none focus:border-accent"
                {...power.register('confirmHostname')}
              />
              {power.errors.confirmHostname && (
                <p className="mt-1 text-[11px] text-danger" role="alert">
                  {power.errors.confirmHostname.message}
                </p>
              )}
            </div>
            {power.errorMessage && (
              <p className="text-xs text-danger" role="alert">
                {power.errorMessage}
              </p>
            )}
            <div className="flex gap-2">
              <Button type="submit" variant="danger" size="sm" loading={power.isPending}>
                Confirmer : {power.selected.label}
              </Button>
              <Button size="sm" onClick={power.cancel} disabled={power.isPending}>
                Annuler
              </Button>
            </div>
          </form>
        )}
        {power.doneMessage && (
          <p className="text-xs text-warning" role="status">
            {power.doneMessage}
          </p>
        )}
      </CardBody>
    </Card>
  )
}
