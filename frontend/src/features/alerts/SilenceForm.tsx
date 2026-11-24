import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Select } from '../../components/ui/Select'
import { FormField, inputClass } from './FormField'
import type { SilenceFormModel } from './useSilenceForm'

/** Poses a silence: the alerts stay open and listed, only their notifications stop. */
export function SilenceForm({ model }: { model: SilenceFormModel }) {
  const { register, errors } = model
  return (
    <Card>
      <CardHeader title={model.title} subtitle={model.subject} />
      <CardBody>
        <form className="grid gap-3 sm:grid-cols-2" onSubmit={model.onSubmit} noValidate>
          {model.scopes.length > 1 && (
            <FormField id="silenceScope" label="Portée" className="sm:col-span-2">
              <Select id="silenceScope" containerClassName="w-full" {...register('scope')}>
                {model.scopes.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </Select>
            </FormField>
          )}
          <FormField id="silenceDuration" label="Durée">
            <Select
              id="silenceDuration"
              containerClassName="w-full"
              {...register('durationMinutes')}
            >
              {model.durations.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </Select>
          </FormField>
          <FormField id="silenceReason" label="Motif" error={errors.reason?.message}>
            <input
              id="silenceReason"
              className={inputClass}
              placeholder="ex. maintenance du disque"
              {...register('reason')}
            />
          </FormField>
          <div className="flex flex-wrap items-center gap-2 sm:col-span-2">
            <Button type="submit" size="sm" variant="primary" loading={model.isSaving}>
              Poser le silence
            </Button>
            <Button size="sm" onClick={model.close}>
              Annuler
            </Button>
            <span className="text-[11px] text-text-muted">
              Les alertes restent ouvertes et visibles ; seules leurs notifications sont suspendues.
            </span>
          </div>
          {model.error && (
            <p className="text-xs text-danger sm:col-span-2" role="alert">
              {model.error}
            </p>
          )}
        </form>
      </CardBody>
    </Card>
  )
}
