import { KeyRound, Save } from 'lucide-react'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { LoadingState } from '../../components/ui/LoadingState'
import { FormField, inputClass } from '../alerts/FormField'
import { HostKeyField } from './HostKeyField'
import type { BackupConfigFormModel } from './useBackupConfigForm'

const textareaClass =
  'min-h-[96px] w-full rounded-md border border-line bg-surface px-3 py-2 font-mono text-xs text-text-primary outline-none focus:border-accent'

/** Repository, sources, exclusions and schedule; read-only for non-administrators. */
export function BackupConfigForm({ model }: { model: BackupConfigFormModel }) {
  if (!model.loaded) return <LoadingState label="Lecture de la configuration…" rows={3} />
  const readOnly = !model.canManage
  return (
    <div className="space-y-4">
      {model.password && <PasswordReveal model={model} />}
      <form onSubmit={model.onSubmit} noValidate className="space-y-4">
        <fieldset disabled={readOnly || model.isSaving} className="min-w-0 space-y-4">
          <Card>
            <CardHeader title="Dépôt" subtitle="Où restic range les instantanés, chiffrés" />
            <CardBody className="grid gap-4 md:grid-cols-2">
              <FormField id="backup-type" label="Type" error={model.errors.type?.message}>
                <select id="backup-type" className={inputClass} {...model.register('type')}>
                  {model.typeOptions.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </FormField>
              <FormField
                id="backup-location"
                label="Emplacement"
                error={model.errors.location?.message}
                hint={model.locationHint}
              >
                <input
                  id="backup-location"
                  className={inputClass}
                  {...model.register('location')}
                />
              </FormField>
              {model.isSFTP && (
                <HostKeyField
                  id="backup-host-key"
                  field={model.register('hostKey')}
                  error={model.errors.hostKey?.message}
                  fingerprint={model.hostKeyFingerprint}
                />
              )}
              {model.credentials && (
                <>
                  <FormField
                    id="backup-username"
                    label={model.credentials.username}
                    error={model.errors.username?.message}
                  >
                    <input
                      id="backup-username"
                      autoComplete="off"
                      placeholder={model.usernameSet ? 'Défini — laisser vide pour conserver' : ''}
                      className={inputClass}
                      {...model.register('username')}
                    />
                  </FormField>
                  <FormField
                    id="backup-secret"
                    label={model.credentials.secret}
                    error={model.errors.secret?.message}
                  >
                    <input
                      id="backup-secret"
                      type="password"
                      autoComplete="new-password"
                      placeholder={model.secretSet ? 'Défini — laisser vide pour conserver' : ''}
                      className={inputClass}
                      {...model.register('secret')}
                    />
                  </FormField>
                </>
              )}
            </CardBody>
          </Card>

          <Card>
            <CardHeader title="Sources et exclusions" subtitle="Une entrée par ligne" />
            <CardBody className="grid gap-4 md:grid-cols-2">
              <FormField
                id="backup-sources"
                label="Chemins sauvegardés"
                error={model.errors.sources?.message}
                hint="Chemins absolus, par exemple /etc ou /var/lib/docker/volumes"
              >
                <textarea
                  id="backup-sources"
                  className={textareaClass}
                  {...model.register('sources')}
                />
              </FormField>
              <FormField
                id="backup-excludes"
                label="Motifs exclus"
                error={model.errors.excludes?.message}
                hint="Motifs restic : *.tmp, **/node_modules…"
              >
                <textarea
                  id="backup-excludes"
                  className={textareaClass}
                  {...model.register('excludes')}
                />
              </FormField>
              <label className="flex items-center gap-2 text-xs text-text-secondary">
                <input type="checkbox" {...model.register('excludeCaches')} />
                Ignorer les répertoires de cache (CACHEDIR.TAG)
              </label>
              <FormField
                id="backup-marker"
                label="Ignorer un répertoire qui contient le fichier"
                error={model.errors.excludeIfPresent?.message}
                hint="Par exemple .nobackup ; vide pour ne rien ignorer"
              >
                <input
                  id="backup-marker"
                  className={inputClass}
                  {...model.register('excludeIfPresent')}
                />
              </FormField>
              <FormField
                id="backup-sqlite"
                label="Bases SQLite à copier avant la sauvegarde"
                error={model.errors.sqliteDatabases?.message}
                hint="Un chemin absolu par ligne, 20 au plus. Chaque base est copiée de façon cohérente (VACUUM INTO) pendant que l'application tourne, puis sauvegardée sous /var/lib/monoops-backup/dumps ; une copie impossible donne un avertissement."
              >
                <textarea
                  id="backup-sqlite"
                  rows={3}
                  className={textareaClass}
                  placeholder="/var/lib/docker/volumes/vaultwarden/_data/db.sqlite3"
                  {...model.register('sqliteDatabases')}
                />
              </FormField>
            </CardBody>
          </Card>

          <Card>
            <CardHeader title="Planification" subtitle="Lancée par le Manager" />
            <CardBody className="grid gap-4 md:grid-cols-4">
              <p className="text-xs text-text-secondary md:col-span-4">
                À l&apos;heure de la machine. Une sauvegarde manquée pendant un arrêt du Manager est
                lancée une fois à son redémarrage.
              </p>
              <label className="flex items-center gap-2 text-xs text-text-secondary md:col-span-4">
                <input type="checkbox" {...model.register('scheduleEnabled')} />
                Sauvegarder automatiquement
              </label>
              <FormField id="backup-frequency" label="Fréquence">
                <select
                  id="backup-frequency"
                  className={inputClass}
                  disabled={!model.scheduleEnabled}
                  {...model.register('frequency')}
                >
                  <option value="daily">Chaque jour</option>
                  <option value="weekly">Chaque semaine</option>
                </select>
              </FormField>
              {model.showWeekday && (
                <FormField id="backup-weekday" label="Jour">
                  <select
                    id="backup-weekday"
                    className={inputClass}
                    disabled={!model.scheduleEnabled}
                    {...model.register('weekday', { valueAsNumber: true })}
                  >
                    {model.weekdayOptions.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </select>
                </FormField>
              )}
              <FormField id="backup-time" label="Heure" error={model.errors.time?.message}>
                <input
                  id="backup-time"
                  type="time"
                  className={inputClass}
                  disabled={!model.scheduleEnabled}
                  {...model.register('time')}
                />
              </FormField>
            </CardBody>
          </Card>
          <Card>
            <CardHeader
              title="Rétention et vérification"
              subtitle="Après chaque sauvegarde planifiée"
            />
            <CardBody className="grid gap-4 md:grid-cols-4">
              <p className="text-xs text-text-secondary md:col-span-4">
                La rétention supprime les instantanés au-delà de la politique (restic forget
                --prune) ; la vérification relit une part des données du dépôt au plus une fois par
                semaine (restic check).
              </p>
              <label className="flex items-center gap-2 text-xs text-text-secondary md:col-span-4">
                <input type="checkbox" {...model.register('retentionEnabled')} />
                Appliquer la rétention
              </label>
              {(
                [
                  ['keepDaily', 'Quotidiens gardés'],
                  ['keepWeekly', 'Hebdomadaires gardés'],
                  ['keepMonthly', 'Mensuels gardés'],
                  ['keepYearly', 'Annuels gardés'],
                ] as const
              ).map(([name, label]) => (
                <FormField
                  key={name}
                  id={`backup-${name}`}
                  label={label}
                  error={model.errors[name]?.message}
                >
                  <input
                    id={`backup-${name}`}
                    type="number"
                    min={0}
                    max={1000}
                    className={inputClass}
                    disabled={!model.retentionEnabled}
                    {...model.register(name, { valueAsNumber: true })}
                  />
                </FormField>
              ))}
              <label className="flex items-center gap-2 text-xs text-text-secondary md:col-span-3">
                <input type="checkbox" {...model.register('checkEnabled')} />
                Vérifier le dépôt chaque semaine
              </label>
              <FormField
                id="backup-subset"
                label="Part relue (%)"
                error={model.errors.checkSubsetPercent?.message}
              >
                <input
                  id="backup-subset"
                  type="number"
                  min={1}
                  max={100}
                  className={inputClass}
                  disabled={!model.checkEnabled}
                  {...model.register('checkSubsetPercent', { valueAsNumber: true })}
                />
              </FormField>
            </CardBody>
          </Card>
        </fieldset>

        {!readOnly && (
          <div className="flex flex-wrap items-center gap-3">
            <Button
              type="submit"
              variant="primary"
              icon={<Save className="h-3.5 w-3.5" />}
              loading={model.isSaving}
            >
              Enregistrer
            </Button>
            {model.saved && (
              <p className="text-xs text-success" role="status">
                Configuration enregistrée.
              </p>
            )}
            {model.saveError && (
              <p className="text-xs text-danger" role="alert">
                {model.saveError}
              </p>
            )}
          </div>
        )}
        {readOnly && (
          <p className="text-xs text-text-muted">
            Seuls les administrateurs modifient la configuration.
          </p>
        )}
      </form>

      {model.canInit && (
        <Card>
          <CardHeader
            title="Initialiser le dépôt"
            subtitle="restic crée le dépôt chiffré à l'emplacement enregistré"
            icon={<KeyRound className="h-3.5 w-3.5" />}
          />
          <CardBody className="space-y-3">
            <p className="text-xs text-text-secondary">
              Le Manager génère le mot de passe du dépôt et l&apos;affiche une seule fois. Sans lui,
              les sauvegardes sont illisibles : gardez-en une copie hors ligne.
            </p>
            {!model.secretsAvailable && (
              <p className="text-xs text-warning" role="status">
                La clé du coffre est absente : le mot de passe ne peut pas être conservé (voir le
                README).
              </p>
            )}
            {model.initError && (
              <p className="text-xs text-danger" role="alert">
                {model.initError}
              </p>
            )}
            <Button
              variant="primary"
              size="sm"
              loading={model.isInitializing}
              disabled={!model.secretsAvailable}
              onClick={model.init}
            >
              Initialiser le dépôt
            </Button>
            <div className="space-y-2 border-t border-line pt-3">
              <p className="text-xs text-text-secondary">
                Le dépôt existe déjà (réinstallation, autre machine) ? Saisissez son mot de passe
                pour le rattacher : restic en liste les instantanés.
              </p>
              <div className="flex flex-wrap gap-2">
                <input
                  type="password"
                  autoComplete="off"
                  aria-label="Mot de passe du dépôt existant"
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
          </CardBody>
        </Card>
      )}

      {model.canMaintain && (
        <Card>
          <CardHeader title="Maintenance" subtitle="Opérations ponctuelles sur le dépôt" />
          <CardBody className="space-y-3">
            <div className="flex flex-wrap gap-2">
              <Button
                size="sm"
                loading={model.operationPending === 'forget-prune'}
                disabled={model.operationPending !== null}
                onClick={() => model.runOperation('forget-prune')}
              >
                Appliquer la rétention
              </Button>
              <Button
                size="sm"
                loading={model.operationPending === 'check'}
                disabled={model.operationPending !== null}
                onClick={() => model.runOperation('check')}
              >
                Vérifier le dépôt
              </Button>
              <Button
                size="sm"
                variant="danger"
                disabled={model.operationPending !== null || model.confirmUnlock}
                onClick={model.askUnlock}
              >
                Forcer le déverrouillage
              </Button>
            </div>
            {model.confirmUnlock && (
              <div
                className="space-y-2 rounded-md border border-danger/30 bg-danger/5 p-3"
                role="alert"
              >
                <p className="text-xs text-text-secondary">
                  Le déverrouillage supprime tous les verrous du dépôt. Si une autre machine ou une
                  opération restic utilise le dépôt en ce moment, il peut être endommagé.
                </p>
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    variant="danger"
                    loading={model.operationPending === 'unlock'}
                    onClick={model.unlock}
                  >
                    Confirmer le déverrouillage
                  </Button>
                  <Button size="sm" onClick={model.cancelUnlock}>
                    Annuler
                  </Button>
                </div>
              </div>
            )}
            {model.operationDone && (
              <p className="text-xs text-text-secondary" role="status">
                Opération lancée : suivez-la dans l&apos;historique.
              </p>
            )}
            {model.operationError && (
              <p className="text-xs text-danger" role="alert">
                {model.operationError}
              </p>
            )}
          </CardBody>
        </Card>
      )}
    </div>
  )
}

function PasswordReveal({ model }: { model: BackupConfigFormModel }) {
  return (
    <div className="space-y-2 rounded-lg border border-warning/40 bg-warning/5 p-4" role="alert">
      <p className="text-sm font-semibold text-text-primary">Mot de passe du dépôt</p>
      <p className="text-xs text-text-secondary">
        Notez-le maintenant et conservez-en une copie hors ligne : il ne sera plus jamais affiché.
        Sans lui, aucune restauration n&apos;est possible si la machine est perdue.
      </p>
      <code className="block break-all rounded bg-surface px-3 py-2 font-mono text-sm text-text-primary">
        {model.password}
      </code>
      <Button size="sm" onClick={model.dismissPassword}>
        J&apos;ai conservé le mot de passe
      </Button>
    </div>
  )
}
