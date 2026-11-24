import { Copy, KeyRound, ShieldCheck } from 'lucide-react'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { QrCode } from '../../components/ui/QrCode'
import type { SecondFactorModel } from './useSecondFactor'

const labelClass = 'mb-1.5 block font-mono text-[10px] uppercase tracking-wider text-text-secondary'
const inputClass =
  'h-9 w-full rounded-md border border-line bg-surface px-3 text-sm text-text-primary outline-none focus:border-accent sm:w-64'

const actionLabels = {
  enroll: 'Continuer',
  regenerate: 'Régénérer les codes',
  disable: 'Désactiver',
}

export function SecondFactorCard({ model }: { model: SecondFactorModel }) {
  const status = model.status

  return (
    <Card>
      <CardHeader
        title="Double authentification"
        subtitle="Application TOTP et codes de secours"
        icon={<ShieldCheck className="h-3.5 w-3.5" />}
        actions={
          status && (
            <Badge tone={status.enabled ? 'success' : 'neutral'} dot>
              {status.enabled ? 'Activée' : 'Désactivée'}
            </Badge>
          )
        }
      />
      <CardBody className="space-y-4">
        {model.state.isError && (
          <ErrorState message={model.state.error.message} onRetry={model.state.refetch} />
        )}
        {model.state.isLoading && <LoadingState label="Chargement…" rows={1} />}

        {model.recoveryCodes && (
          <div className="space-y-3 rounded-md border border-warning/40 bg-warning/5 p-4">
            <p className="text-sm font-medium text-text-primary">Vos codes de secours</p>
            <p className="text-xs text-text-secondary">
              Chaque code sert une seule fois, si l'application n'est pas disponible. Conservez-les
              hors de cet ordinateur : ils ne seront plus affichés.
            </p>
            <ul className="grid grid-cols-2 gap-x-6 gap-y-1 font-mono text-sm text-text-primary sm:w-80">
              {model.recoveryCodes.map((code) => (
                <li key={code}>{code}</li>
              ))}
            </ul>
            <div className="flex flex-wrap gap-2">
              <Button size="sm" icon={<Copy className="h-3.5 w-3.5" />} onClick={model.copyCodes}>
                {model.copied ? 'Copiés' : 'Copier'}
              </Button>
              <Button size="sm" variant="primary" onClick={model.acknowledgeCodes}>
                J'ai conservé mes codes
              </Button>
            </div>
          </div>
        )}

        {status && !model.recoveryCodes && !model.action && (
          <div className="space-y-3">
            <p className="text-xs leading-relaxed text-text-secondary">
              {status.enabled
                ? `Un code de l'application est demandé après le mot de passe. Codes de secours restants : ${status.recoveryCodesRemaining}.`
                : "Après le mot de passe, la connexion demandera un code à 6 chiffres d'une application d'authentification (Aegis, Google Authenticator, 1Password…)."}
            </p>
            {!status.available && (
              <p className="text-xs text-warning" role="status">
                La clé du coffre de secrets est absente : l'activation est impossible (voir le
                README).
              </p>
            )}
            <div className="flex flex-wrap gap-2">
              {!status.enabled && (
                <Button
                  size="sm"
                  variant="primary"
                  icon={<KeyRound className="h-3.5 w-3.5" />}
                  disabled={!status.available}
                  onClick={() => model.start('enroll')}
                >
                  Activer
                </Button>
              )}
              {status.enabled && (
                <>
                  <Button size="sm" onClick={() => model.start('regenerate')}>
                    Régénérer les codes de secours
                  </Button>
                  <Button size="sm" variant="danger" onClick={() => model.start('disable')}>
                    Désactiver
                  </Button>
                </>
              )}
            </div>
          </div>
        )}

        {model.action && model.askPassword && (
          <form className="space-y-3" onSubmit={model.onPasswordSubmit} noValidate>
            <div>
              <label htmlFor="secondFactorPassword" className={labelClass}>
                Mot de passe actuel
              </label>
              <input
                id="secondFactorPassword"
                type="password"
                autoComplete="current-password"
                className={inputClass}
                {...model.registerPassword('password')}
              />
              {model.passwordErrors.password && (
                <p className="mt-1 text-[11px] text-danger" role="alert">
                  {model.passwordErrors.password.message}
                </p>
              )}
            </div>
            <div className="flex gap-2">
              <Button
                type="submit"
                size="sm"
                variant={model.action === 'disable' ? 'danger' : 'primary'}
                loading={model.isPending}
              >
                {actionLabels[model.action]}
              </Button>
              <Button size="sm" onClick={model.cancel}>
                Annuler
              </Button>
            </div>
          </form>
        )}

        {model.enrollment && (
          <form className="space-y-4" onSubmit={model.onCodeSubmit} noValidate>
            <div className="flex flex-col gap-4 sm:flex-row sm:items-start">
              <QrCode
                path={model.enrollment.path}
                viewBox={model.enrollment.viewBox}
                label="QR code à scanner avec l'application d'authentification"
              />
              <div className="space-y-2 text-xs text-text-secondary">
                <p>1. Scannez ce QR code avec votre application d'authentification.</p>
                <p>
                  Sinon, saisissez la clé :{' '}
                  <code className="break-all font-mono text-text-primary">
                    {model.enrollment.secret}
                  </code>
                </p>
                <p>2. Saisissez le code à 6 chiffres affiché par l'application.</p>
              </div>
            </div>
            <div>
              <label htmlFor="totpCode" className={labelClass}>
                Code de l'application
              </label>
              <input
                id="totpCode"
                inputMode="numeric"
                autoComplete="one-time-code"
                className={inputClass}
                {...model.registerCode('code')}
              />
              {model.codeErrors.code && (
                <p className="mt-1 text-[11px] text-danger" role="alert">
                  {model.codeErrors.code.message}
                </p>
              )}
            </div>
            <div className="flex gap-2">
              <Button type="submit" size="sm" variant="primary" loading={model.isPending}>
                Vérifier et activer
              </Button>
              <Button size="sm" onClick={model.cancel}>
                Annuler
              </Button>
            </div>
          </form>
        )}

        {model.errorMessage && (
          <p className="text-xs text-danger" role="alert">
            {model.errorMessage}
          </p>
        )}
      </CardBody>
    </Card>
  )
}
