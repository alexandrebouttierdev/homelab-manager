import { Pencil, Plus, Trash2 } from 'lucide-react'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/DataTable'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { Select } from '../../components/ui/Select'
import { FormField as Field, inputClass } from './FormField'
import { useAlertRulesPanel, type AlertRulesPanelModel } from './useAlertRulesPanel'

export function AlertRulesPanel() {
  const page = useAlertRulesPanel()

  return (
    <div className="space-y-4">
      {page.state.isError && (
        <ErrorState message={page.state.error.message} onRetry={page.state.refetch} />
      )}
      {page.state.isLoading && <LoadingState label="Chargement des règles…" rows={3} />}
      {page.actionError && (
        <p className="text-xs text-danger" role="alert">
          {page.actionError}
        </p>
      )}

      {page.canManage && !page.editing && (
        <Button
          size="sm"
          variant="primary"
          icon={<Plus className="h-3.5 w-3.5" />}
          onClick={page.startCreate}
        >
          Nouvelle règle
        </Button>
      )}
      {!page.canManage && (
        <p className="text-xs text-text-secondary">
          Seuls les administrateurs modifient les règles.
        </p>
      )}

      {page.editing && <RuleForm page={page} />}

      {page.pendingDeletion && (
        <Card className="border-danger/40">
          <CardBody className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm text-text-primary">
              Supprimer la règle « {page.pendingDeletion.name} » ? Ses alertes ouvertes sont
              résolues ; leur historique est conservé.
            </p>
            <div className="flex gap-2">
              <Button size="sm" onClick={page.cancelDelete}>
                Annuler
              </Button>
              <Button
                size="sm"
                variant="danger"
                loading={page.isDeleting}
                onClick={page.confirmDelete}
              >
                Confirmer la suppression
              </Button>
            </div>
          </CardBody>
        </Card>
      )}

      {page.rows.length > 0 && (
        <Table>
          <THead>
            <TR>
              <TH>Règle</TH>
              <TH>Condition</TH>
              <TH>Sévérité</TH>
              <TH>État</TH>
              <TH className="relative">
                <span className="sr-only">Actions</span>
              </TH>
            </TR>
          </THead>
          <TBody>
            {page.rows.map((row) => (
              <TR key={row.id}>
                <TD>
                  <p className="text-text-primary">{row.name}</p>
                  <p className="text-[11px] text-text-muted">
                    {row.type}
                    {row.isDefault && ' · par défaut'}
                  </p>
                </TD>
                <TD className="text-xs">{row.description}</TD>
                <TD>
                  <Badge tone={row.severity.tone} dot>
                    {row.severity.label}
                  </Badge>
                </TD>
                <TD>
                  {page.canManage ? (
                    <Button
                      size="sm"
                      disabled={page.isToggling}
                      aria-pressed={row.enabled}
                      onClick={() => page.toggleEnabled(row.id)}
                    >
                      {row.enabled ? 'Active' : 'Désactivée'}
                    </Button>
                  ) : (
                    <Badge tone={row.enabled ? 'success' : 'neutral'}>
                      {row.enabled ? 'Active' : 'Désactivée'}
                    </Badge>
                  )}
                </TD>
                <TD className="whitespace-nowrap text-right">
                  {page.canManage && (
                    <div className="flex justify-end gap-1">
                      <Button
                        size="sm"
                        aria-label={`Modifier la règle ${row.name}`}
                        icon={<Pencil className="h-3.5 w-3.5" />}
                        onClick={() => page.startEdit(row.id)}
                      />
                      {!row.isDefault && (
                        <Button
                          size="sm"
                          aria-label={`Supprimer la règle ${row.name}`}
                          icon={<Trash2 className="h-3.5 w-3.5" />}
                          onClick={() => page.askDelete(row)}
                        />
                      )}
                    </div>
                  )}
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      )}
    </div>
  )
}

function RuleForm({ page }: { page: AlertRulesPanelModel }) {
  const { register, errors, options, type } = page
  const metricRule = type === 'host_metric' || type === 'container_metric'
  const containerRule = type === 'container_metric' || type === 'container_state'

  return (
    <Card>
      <CardHeader title={page.isNew ? 'Nouvelle règle' : 'Modifier la règle'} />
      <CardBody>
        <form
          className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3"
          onSubmit={page.onSave}
          noValidate
        >
          <Field id="ruleName" label="Nom" error={errors.name?.message}>
            <input id="ruleName" className={inputClass} {...register('name')} />
          </Field>
          <Field id="ruleType" label="Type">
            <Select id="ruleType" containerClassName="w-full" {...page.registerType}>
              {options.types.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </Select>
          </Field>
          <Field id="ruleSeverity" label="Sévérité">
            <Select id="ruleSeverity" containerClassName="w-full" {...register('severity')}>
              {options.severities.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </Select>
          </Field>

          {containerRule && (
            <Field id="ruleContainer" label="Conteneur">
              <Select id="ruleContainer" containerClassName="w-full" {...register('containerId')}>
                {options.containers.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </Select>
            </Field>
          )}

          {metricRule && (
            <>
              <Field id="ruleMetric" label="Métrique" error={errors.metric?.message}>
                <Select id="ruleMetric" containerClassName="w-full" {...page.registerMetric}>
                  {options.metrics.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </Select>
              </Field>
              <Field id="ruleLabelKey" label="Limiter à">
                <Select id="ruleLabelKey" containerClassName="w-full" {...register('labelKey')}>
                  {options.labels.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </Select>
              </Field>
              <Field id="ruleLabelValue" label="Valeur du label" error={errors.labelValue?.message}>
                <input
                  id="ruleLabelValue"
                  className={inputClass}
                  placeholder="ex. /var/lib/docker"
                  {...register('labelValue')}
                />
              </Field>
              <Field id="ruleOperator" label="Opérateur">
                <Select id="ruleOperator" containerClassName="w-full" {...register('operator')}>
                  {options.operators.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </Select>
              </Field>
              <Field id="ruleThreshold" label="Seuil" error={errors.threshold?.message}>
                <input
                  id="ruleThreshold"
                  inputMode="decimal"
                  className={inputClass}
                  {...register('threshold')}
                />
              </Field>
            </>
          )}

          {type === 'container_state' && (
            <fieldset className="sm:col-span-2 xl:col-span-3">
              <legend className="mb-1.5 font-mono text-[10px] uppercase tracking-wider text-text-secondary">
                États déclencheurs
              </legend>
              <div className="flex flex-wrap gap-3">
                {options.states.map((option) => (
                  <label key={option.value} className="flex items-center gap-1.5 text-sm">
                    <input type="checkbox" value={option.value} {...register('states')} />
                    {option.label}
                  </label>
                ))}
              </div>
              {errors.states?.message && (
                <p className="mt-1 text-[11px] text-danger">{errors.states.message}</p>
              )}
            </fieldset>
          )}

          {type === 'unit_failed' && (
            <Field id="ruleUnitPattern" label="Unité ou motif" error={errors.unitPattern?.message}>
              <input
                id="ruleUnitPattern"
                className={inputClass}
                placeholder="* ou backup-*.service"
                {...register('unitPattern')}
              />
            </Field>
          )}

          {type === 'collector' && (
            <Field id="ruleCollector" label="Collecteur" error={errors.collector?.message}>
              <Select id="ruleCollector" containerClassName="w-full" {...register('collector')}>
                {options.collectors.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </Select>
            </Field>
          )}

          {type === 'vulnerabilities' && (
            <>
              <Field id="ruleVulnSeverity" label="Sévérité minimale">
                <Select
                  id="ruleVulnSeverity"
                  containerClassName="w-full"
                  {...register('vulnSeverity')}
                >
                  {options.vulnSeverities.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </Select>
              </Field>
              <label className="flex items-center gap-2 self-end text-sm">
                <input type="checkbox" {...register('fixableOnly')} />
                Corrigeables seulement
              </label>
            </>
          )}

          {type === 'backup' && (
            <Field
              id="ruleMaxAge"
              label="Dernier succès plus ancien que (heures)"
              error={errors.maxAgeHours?.message}
            >
              <input
                id="ruleMaxAge"
                inputMode="numeric"
                className={inputClass}
                {...register('maxAgeHours')}
              />
            </Field>
          )}

          {type !== 'unit_failed' && type !== 'vulnerabilities' && type !== 'backup' && (
            <Field id="ruleDuration" label="Pendant (minutes)" error={errors.forMinutes?.message}>
              <input
                id="ruleDuration"
                inputMode="decimal"
                className={inputClass}
                {...register('forMinutes')}
              />
            </Field>
          )}

          <label className="flex items-center gap-2 self-end text-sm">
            <input type="checkbox" {...register('enabled')} />
            Règle active
          </label>

          <div className="flex flex-wrap items-center gap-2 sm:col-span-2 xl:col-span-3">
            <Button type="submit" size="sm" variant="primary" loading={page.isSaving}>
              Enregistrer
            </Button>
            <Button size="sm" onClick={page.cancelEdit}>
              Annuler
            </Button>
            {!page.isNew && (
              <span className="text-[11px] text-text-muted">
                Modifier la condition résout les alertes ouvertes de la règle ; elles se
                redéclenchent si la nouvelle condition est vraie.
              </span>
            )}
          </div>
          {page.saveError && (
            <p className="text-xs text-danger sm:col-span-2 xl:col-span-3" role="alert">
              {page.saveError}
            </p>
          )}
        </form>
      </CardBody>
    </Card>
  )
}
