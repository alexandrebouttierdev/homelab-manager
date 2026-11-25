import { Server } from 'lucide-react'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardFooter, CardHeader } from '../../components/ui/Card'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { FormField, inputClass } from '../alerts/FormField'
import { useSystemPreferences } from './useSystemPreferences'

/** Host name, time zone and language of the machine (D-6). */
export function SystemPreferencesCard() {
  const model = useSystemPreferences()
  return (
    <Card>
      <CardHeader
        title="Préférences système"
        subtitle="Nom de la machine, fuseau horaire et langue"
        icon={<Server className="h-3.5 w-3.5" />}
      />
      <CardBody className="space-y-3">
        {model.state.isLoading && <LoadingState label="Lecture des préférences…" rows={2} />}
        {model.state.isError && (
          <ErrorState message={model.state.error.message} onRetry={model.state.refetch} />
        )}
        {model.preferences && (
          <form onSubmit={model.onSubmit} className="space-y-3" noValidate>
            <div className="grid gap-3 sm:grid-cols-3">
              <FormField
                id="system-hostname"
                label="Nom de la machine"
                error={model.errors.hostname?.message}
              >
                <input
                  id="system-hostname"
                  className={inputClass}
                  readOnly={!model.editable}
                  autoComplete="off"
                  {...model.register('hostname')}
                />
              </FormField>
              <FormField
                id="system-timezone"
                label="Fuseau horaire"
                error={model.errors.timezone?.message}
              >
                <input
                  id="system-timezone"
                  className={inputClass}
                  readOnly={!model.editable}
                  list="system-timezones"
                  autoComplete="off"
                  {...model.register('timezone')}
                />
                <datalist id="system-timezones">
                  {model.timezones.map((zone) => (
                    <option key={zone} value={zone} />
                  ))}
                </datalist>
              </FormField>
              <FormField
                id="system-locale"
                label="Langue du système"
                error={model.errors.locale?.message}
              >
                <input
                  id="system-locale"
                  className={inputClass}
                  readOnly={!model.localeEditable}
                  list="system-locales"
                  autoComplete="off"
                  {...model.register('locale')}
                />
                <datalist id="system-locales">
                  {model.locales.map((locale) => (
                    <option key={locale} value={locale} />
                  ))}
                </datalist>
              </FormField>
            </div>
            {model.localeLocked && (
              <p className="text-xs text-text-muted">
                La distribution interdit de changer la langue par systemd-localed (Debian) :
                utilisez <code>sudo dpkg-reconfigure locales</code> sur la machine.
              </p>
            )}
            {model.confirmation && (
              <div
                className="space-y-2 rounded-md border border-warning/40 bg-warning/5 p-3"
                role="group"
                aria-label="Confirmer les changements"
              >
                <ul className="space-y-1.5 text-xs text-text-secondary">
                  {model.confirmation.map((change) => (
                    <li key={change.label}>
                      <span className="font-medium text-text-primary">{change.label}</span> :{' '}
                      <code>{change.from || '—'}</code> → <code>{change.to}</code>
                      <span className="block text-xs text-text-muted">{change.consequence}.</span>
                    </li>
                  ))}
                </ul>
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    variant="primary"
                    loading={model.isSaving}
                    onClick={model.confirm}
                  >
                    Confirmer les changements
                  </Button>
                  <Button size="sm" onClick={model.cancel} disabled={model.isSaving}>
                    Annuler
                  </Button>
                </div>
              </div>
            )}
            {model.errorMessage && (
              <p className="text-xs text-danger" role="alert">
                {model.errorMessage}
              </p>
            )}
            {model.saved && (
              <p className="text-xs text-success" role="status">
                Modifié : {model.saved}.
              </p>
            )}
            {model.editable && !model.confirmation && (
              <Button type="submit" size="sm" variant="primary" disabled={!model.isDirty}>
                Enregistrer
              </Button>
            )}
          </form>
        )}
      </CardBody>
      <CardFooter className="text-xs text-text-muted">
        {model.disabledNotice
          ? 'Modification désactivée dans la configuration du Manager (system_preferences_enabled) ; la règle polkit 50-homelab-manager-system.rules doit aussi être installée.'
          : model.editable
            ? 'Appliqué par systemd-hostnamed, -timedated et -localed ; chaque changement est inscrit au journal d’audit.'
            : 'Lecture seule : seuls les administrateurs modifient ces préférences.'}
      </CardFooter>
    </Card>
  )
}
