import { Save, ShieldCheck } from 'lucide-react'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { useSecurityPolicyPanel } from './useSecurityPolicyPanel'

const labelClass = 'mb-1.5 block text-xs text-text-secondary'
const inputClass =
  'w-full rounded-md border border-line bg-surface px-3 text-sm text-text-primary outline-none focus:border-accent'

export function SecurityPolicyPanel() {
  const policy = useSecurityPolicyPanel()

  return (
    <Card>
      <CardHeader
        title="Politique de sécurité"
        subtitle="Inactivité des sessions et réseaux autorisés"
        icon={<ShieldCheck className="h-3.5 w-3.5" />}
      />
      <CardBody>
        {policy.state.isError && (
          <ErrorState message={policy.state.error.message} onRetry={policy.state.refetch} />
        )}
        {policy.state.isLoading && <LoadingState label="Chargement de la politique…" rows={2} />}
        {policy.loaded && (
          <form className="space-y-4" onSubmit={policy.onSubmit} noValidate>
            <div>
              <label htmlFor="idleTimeoutMinutes" className={labelClass}>
                Expiration après inactivité (minutes)
              </label>
              <input
                id="idleTimeoutMinutes"
                type="number"
                min={5}
                max={720}
                step={1}
                className={`${inputClass} h-9 sm:w-40`}
                aria-invalid={Boolean(policy.errors.idleTimeoutMinutes)}
                {...policy.register('idleTimeoutMinutes', { valueAsNumber: true })}
              />
              {policy.errors.idleTimeoutMinutes && (
                <p className="mt-1 text-xs text-danger" role="alert">
                  {policy.errors.idleTimeoutMinutes.message}
                </p>
              )}
            </div>
            <div>
              <label htmlFor="networks" className={labelClass}>
                Réseaux autorisés (un préfixe CIDR par ligne, vide = toutes les adresses)
              </label>
              <textarea
                id="networks"
                rows={4}
                spellCheck={false}
                className={`${inputClass} py-2 font-mono`}
                aria-invalid={Boolean(policy.errors.networks)}
                {...policy.register('networks')}
              />
              {policy.errors.networks && (
                <p className="mt-1 text-xs text-danger" role="alert">
                  {policy.errors.networks.message}
                </p>
              )}
              <p className="mt-1 text-xs text-text-muted">
                Votre adresse vue par le Manager : <code>{policy.currentAddress}</code>. Une liste
                qui l'exclut est refusée. Derrière un proxy inverse, c'est l'adresse du proxy.
              </p>
            </div>
            <label className="flex cursor-pointer items-start gap-2 text-sm text-text-primary">
              <input
                type="checkbox"
                className="mt-1 h-3.5 w-3.5 accent-accent"
                {...policy.register('requireAdminSecondFactor')}
              />
              <span>
                Exiger la double authentification des administrateurs
                <span className="block text-xs text-text-muted">
                  Un administrateur sans second facteur ne pourra que l'activer. Activez d'abord le
                  vôtre dans Paramètres.
                </span>
              </span>
            </label>
            <label className="flex cursor-pointer items-start gap-2 text-sm text-text-primary">
              <input
                type="checkbox"
                className="mt-1 h-3.5 w-3.5 accent-accent"
                {...policy.register('allowPasskeyLogin')}
              />
              <span>
                Autoriser la connexion par passkey seule
                <span className="block text-xs text-text-muted">
                  Un compte qui a enregistré une passkey pourra se connecter sans son mot de passe,
                  avec le code ou la biométrie de son appareil. Exige le domaine WebAuthn du
                  Manager.
                </span>
              </span>
            </label>
            {policy.errorMessage && (
              <p className="text-xs text-danger" role="alert">
                {policy.errorMessage}
              </p>
            )}
            <div className="flex items-center gap-3">
              <Button
                type="submit"
                variant="primary"
                loading={policy.isPending}
                icon={<Save className="h-3.5 w-3.5" />}
              >
                Enregistrer
              </Button>
              {policy.saved && (
                <p className="text-xs text-success" role="status">
                  Politique enregistrée.
                </p>
              )}
            </div>
            <p className="text-xs text-text-muted">
              Après 5 mots de passe faux consécutifs, un compte est verrouillé 15 minutes.
            </p>
          </form>
        )}
      </CardBody>
    </Card>
  )
}
