import { Copy, Save } from 'lucide-react'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { formatDateTime, formatRelativeTime } from '../../lib/format'
import { FormField, inputClass } from '../alerts/FormField'
import type { BackupCopyModel } from './useBackupCopy'

/** Copy repository (restic copy): a second place for the snapshots, for the 3-2-1 rule. */
export function BackupCopyCard({ model }: { model: BackupCopyModel }) {
  if (!model.visible) return null
  const readOnly = !model.canManage
  return (
    <Card>
      <CardHeader
        title="Dépôt de copie"
        subtitle="Les instantanés copiés vers un second dépôt (restic copy)"
        icon={<Copy className="h-3.5 w-3.5" />}
      />
      <CardBody className="space-y-4">
        <p className="text-xs text-text-secondary">
          Pour la règle 3-2-1 : un second dépôt, de préférence hors de la machine, reçoit une copie
          des instantanés. Il a son propre mot de passe. La rétention du dépôt principal
          s&apos;applique aussi à la copie.
        </p>
        {model.copy && (
          <div className="flex flex-wrap items-center gap-2 text-xs text-text-secondary">
            <Badge tone={model.copy.initialized ? 'success' : 'warning'} dot>
              {model.copy.initialized ? 'Prêt' : 'À initialiser ou rattacher'}
            </Badge>
            <span className="break-all font-mono">{model.copy.label}</span>
            {model.copy.initializedAt && (
              <span>depuis le {formatDateTime(model.copy.initializedAt)}</span>
            )}
          </div>
        )}
        {model.lastCopy && (
          <div className="text-xs text-text-secondary" role="status">
            <span>Dernière copie {formatRelativeTime(model.lastCopy.startedAt)} : </span>
            <Badge tone={model.lastCopy.state.tone}>{model.lastCopy.state.label}</Badge>
            {model.lastCopy.result && <span> {model.lastCopy.result}</span>}
            {model.lastCopy.problem && <p className="text-danger">{model.lastCopy.problem}</p>}
          </div>
        )}
        {model.password && (
          <div
            className="space-y-2 rounded-lg border border-warning/40 bg-warning/5 p-4"
            role="alert"
          >
            <p className="text-sm font-semibold text-text-primary">
              Mot de passe du dépôt de copie
            </p>
            <p className="text-xs text-text-secondary">
              Notez-le maintenant et conservez-en une copie hors ligne : il ne sera plus jamais
              affiché.
            </p>
            <code className="block break-all rounded bg-surface px-3 py-2 font-mono text-sm text-text-primary">
              {model.password}
            </code>
            <Button size="sm" onClick={model.dismissPassword}>
              J&apos;ai conservé le mot de passe
            </Button>
          </div>
        )}

        <form onSubmit={model.onSubmit} noValidate className="space-y-3">
          <fieldset
            disabled={readOnly || model.isSaving}
            className="grid min-w-0 gap-4 md:grid-cols-2"
          >
            <FormField id="copy-type" label="Type" error={model.errors.type?.message}>
              <select id="copy-type" className={inputClass} {...model.register('type')}>
                {model.typeOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </FormField>
            <FormField
              id="copy-location"
              label="Emplacement"
              error={model.errors.location?.message}
              hint="Un autre dépôt que le principal"
            >
              <input id="copy-location" className={inputClass} {...model.register('location')} />
            </FormField>
            {model.credentials && (
              <>
                <FormField
                  id="copy-username"
                  label={model.credentials.username}
                  error={model.errors.username?.message}
                >
                  <input
                    id="copy-username"
                    autoComplete="off"
                    placeholder={model.usernameSet ? 'Défini — laisser vide pour conserver' : ''}
                    className={inputClass}
                    {...model.register('username')}
                  />
                </FormField>
                <FormField
                  id="copy-secret"
                  label={model.credentials.secret}
                  error={model.errors.secret?.message}
                >
                  <input
                    id="copy-secret"
                    type="password"
                    autoComplete="new-password"
                    placeholder={model.secretSet ? 'Défini — laisser vide pour conserver' : ''}
                    className={inputClass}
                    {...model.register('secret')}
                  />
                </FormField>
              </>
            )}
            <label className="flex items-center gap-2 text-xs text-text-secondary md:col-span-2">
              <input type="checkbox" {...model.register('afterBackup')} />
              Copier après chaque sauvegarde planifiée réussie
            </label>
          </fieldset>
          {!readOnly && (
            <div className="flex flex-wrap items-center gap-3">
              <Button
                type="submit"
                size="sm"
                variant="primary"
                icon={<Save className="h-3.5 w-3.5" />}
                loading={model.isSaving}
              >
                Enregistrer le dépôt de copie
              </Button>
              {model.saved && (
                <p className="text-xs text-success" role="status">
                  Dépôt de copie enregistré.
                </p>
              )}
              {model.saveError && (
                <p className="text-xs text-danger" role="alert">
                  {model.saveError}
                </p>
              )}
            </div>
          )}
        </form>

        {model.needsMain && (
          <p className="text-xs text-text-muted">
            Initialisez d&apos;abord le dépôt principal : la copie en lit les instantanés.
          </p>
        )}
        {model.canInit && (
          <div className="space-y-3 border-t border-line pt-3">
            <p className="text-xs text-text-secondary">
              Le Manager crée le dépôt de copie avec les paramètres de découpage du dépôt principal
              (pour garder la déduplication), puis y copie les instantanés. Son mot de passe est
              affiché une seule fois.
            </p>
            {model.initError && (
              <p className="text-xs text-danger" role="alert">
                {model.initError}
              </p>
            )}
            <Button
              size="sm"
              variant="primary"
              loading={model.isInitializing}
              disabled={!model.secretsAvailable}
              onClick={model.init}
            >
              Créer le dépôt de copie
            </Button>
            <p className="text-xs text-text-secondary">
              Le dépôt de copie existe déjà ? Saisissez son mot de passe : une première copie le
              rattache.
            </p>
            <div className="flex flex-wrap gap-2">
              <input
                type="password"
                autoComplete="off"
                aria-label="Mot de passe du dépôt de copie existant"
                className={`${inputClass} max-w-xs`}
                value={model.existingPassword}
                onChange={(event) => model.setExistingPassword(event.target.value)}
              />
              <Button
                size="sm"
                loading={model.isConnecting}
                disabled={!model.canConnect || !model.secretsAvailable}
                onClick={model.connect}
              >
                Rattacher ce dépôt
              </Button>
            </div>
            {model.connectError && (
              <p className="text-xs text-danger" role="alert">
                {model.connectError}
              </p>
            )}
          </div>
        )}
        {(model.canCopy || model.canRemove) && (
          <div className="flex flex-wrap gap-2 border-t border-line pt-3">
            {model.canCopy && (
              <Button size="sm" loading={model.isCopying} onClick={model.copyNow}>
                Copier maintenant
              </Button>
            )}
            {model.canRemove && (
              <Button
                size="sm"
                variant="danger"
                disabled={model.confirmRemove}
                onClick={model.askRemove}
              >
                Retirer le dépôt de copie
              </Button>
            )}
          </div>
        )}
        {model.copyStarted && (
          <p className="text-xs text-text-secondary" role="status">
            Copie lancée : suivez-la dans l&apos;historique.
          </p>
        )}
        {model.copyError && (
          <p className="text-xs text-danger" role="alert">
            {model.copyError}
          </p>
        )}
        {model.confirmRemove && (
          <div
            className="space-y-2 rounded-md border border-danger/30 bg-danger/5 p-3"
            role="alert"
          >
            <p className="text-xs text-text-secondary">
              Le Manager oublie le dépôt de copie et son mot de passe ; les données restent dans le
              dépôt. Pour le rattacher plus tard, il faudra ce mot de passe.
            </p>
            <div className="flex gap-2">
              <Button size="sm" variant="danger" loading={model.isRemoving} onClick={model.remove}>
                Confirmer le retrait
              </Button>
              <Button size="sm" onClick={model.cancelRemove}>
                Annuler
              </Button>
            </div>
          </div>
        )}
        {model.removeError && (
          <p className="text-xs text-danger" role="alert">
            {model.removeError}
          </p>
        )}
      </CardBody>
    </Card>
  )
}
