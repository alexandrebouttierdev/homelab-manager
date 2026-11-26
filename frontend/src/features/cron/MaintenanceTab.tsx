import { Play, Wrench } from 'lucide-react'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardFooter, CardHeader } from '../../components/ui/Card'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { Select } from '../../components/ui/Select'
import { formatDateTime, formatRelativeTime } from '../../lib/format'
import { FormField, inputClass } from '../alerts/FormField'
import { useMaintenanceTasks } from './useMaintenanceTasks'

/** The maintenance catalog: fixed tasks the Manager schedules (D-1). */
export function MaintenanceTab() {
  const model = useMaintenanceTasks()
  return (
    <div className="space-y-4">
      <p className="text-xs text-text-secondary">
        Tâches prédéfinies par MonoOps : seuls leur horaire et leurs paramètres se règlent, aucune
        commande ne se saisit. Chacune tourne dans une unité systemd fournie, à l&apos;heure de la
        machine ; une tâche manquée pendant un arrêt du Manager n&apos;est pas rattrapée.
      </p>
      {model.state.isLoading && <LoadingState label="Lecture des tâches…" rows={2} />}
      {model.state.isError && (
        <ErrorState message={model.state.error.message} onRetry={model.state.refetch} />
      )}
      {model.tasks.map((task) => (
        <Card key={task.kind}>
          <CardHeader
            title={task.title}
            subtitle={task.schedule}
            icon={<Wrench className="h-3.5 w-3.5" />}
            actions={
              model.editable && (
                <Button
                  size="sm"
                  icon={<Play className="h-3.5 w-3.5" />}
                  loading={task.running}
                  disabled={task.running}
                  onClick={task.run}
                >
                  Lancer maintenant
                </Button>
              )
            }
          />
          <CardBody className="space-y-3">
            <p className="text-xs text-text-secondary">{task.description}</p>
            <div className="flex flex-wrap items-center gap-2 text-xs">
              <Badge tone={task.lastRun.tone}>{task.lastRun.label}</Badge>
              {task.lastRunAt && (
                <span className="text-text-muted" title={formatDateTime(task.lastRunAt)}>
                  {formatRelativeTime(task.lastRunAt)}
                </span>
              )}
              {task.nextRunAt && (
                <span className="text-text-muted">
                  Prochaine : {formatDateTime(task.nextRunAt)}
                </span>
              )}
            </div>
            {model.editable ? (
              <div className="grid gap-3 md:grid-cols-5">
                <label className="flex items-center gap-2 text-xs text-text-secondary md:col-span-5">
                  <input
                    type="checkbox"
                    checked={task.draft.schedule.enabled}
                    onChange={(event) => task.setEnabled(event.target.checked)}
                  />
                  Planifiée
                </label>
                <FormField id={`${task.kind}-frequency`} label="Fréquence">
                  <Select
                    id={`${task.kind}-frequency`}
                    value={task.draft.schedule.frequency}
                    onChange={(event) =>
                      task.setFrequency(event.target.value === 'daily' ? 'daily' : 'weekly')
                    }
                  >
                    <option value="daily">Chaque jour</option>
                    <option value="weekly">Chaque semaine</option>
                  </Select>
                </FormField>
                {task.draft.schedule.frequency === 'weekly' && (
                  <FormField id={`${task.kind}-weekday`} label="Jour">
                    <Select
                      id={`${task.kind}-weekday`}
                      value={task.draft.schedule.weekday}
                      onChange={(event) => task.setWeekday(Number(event.target.value))}
                    >
                      {model.weekdayOptions.map((option) => (
                        <option key={option.value} value={option.value}>
                          {option.label}
                        </option>
                      ))}
                    </Select>
                  </FormField>
                )}
                <FormField
                  id={`${task.kind}-time`}
                  label="Heure"
                  error={task.timeError ?? undefined}
                >
                  <input
                    id={`${task.kind}-time`}
                    className={inputClass}
                    value={task.draft.schedule.time}
                    onChange={(event) => task.setTime(event.target.value)}
                  />
                </FormField>
                <FormField id={`${task.kind}-size`} label="Taille maximale du journal">
                  <Select
                    id={`${task.kind}-size`}
                    value={task.draft.maxSizeMiB}
                    onChange={(event) => task.setSize(Number(event.target.value))}
                  >
                    {task.sizeOptions.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </Select>
                </FormField>
                <div className="flex items-end">
                  <Button
                    size="sm"
                    variant="primary"
                    disabled={!task.dirty || task.timeError !== null}
                    loading={model.saving}
                    onClick={task.save}
                  >
                    Enregistrer
                  </Button>
                </div>
              </div>
            ) : (
              <p className="text-xs text-text-muted">Taille maximale : {task.currentSize}.</p>
            )}
            {model.error && (
              <p className="text-xs text-danger" role="alert">
                {model.error}
              </p>
            )}
          </CardBody>
          <CardFooter className="text-xs text-text-muted">Unité : {task.unit}</CardFooter>
        </Card>
      ))}
    </div>
  )
}
