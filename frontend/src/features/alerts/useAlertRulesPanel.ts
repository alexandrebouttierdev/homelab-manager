import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { loadState } from '../../hooks/loadState'
import { useSession } from '../../hooks/useSession'
import {
  alertRuleInput,
  collectorLabel,
  containerStateLabel,
  emptyRuleValues,
  fitMetricValues,
  labelOptions,
  metricOptions,
  operatorOptions,
  ruleToValues,
  ruleTypeOptions,
  severityOptions,
  toggledRuleInput,
  toRuleRow,
  type RuleRow,
} from '../../lib/alerts'
import {
  createAlertRule,
  deleteAlertRule,
  listAlertRules,
  updateAlertRule,
  type AlertRuleInput,
} from '../../lib/api/alerts'
import { listContainers } from '../../lib/api/monitoring'
import { canManageAlerts } from '../../lib/roles'
import { alertRuleSchema, type AlertRuleValues } from '../../schemas/alertRule'
import { alertsQueryKey } from './queryKeys'

const rulesQueryKey = [...alertsQueryKey, 'rules'] as const

/** Logic and view model of the rules tab of AlertsPage; changes are for administrators. */
export function useAlertRulesPanel() {
  const queryClient = useQueryClient()
  const { user } = useSession()
  const canManage = canManageAlerts(user?.role)
  const rulesQuery = useQuery({ queryKey: rulesQueryKey, queryFn: listAlertRules })
  const containersQuery = useQuery({
    queryKey: ['containers', false],
    queryFn: () => listContainers(false),
    enabled: canManage,
  })
  // Every rule change may resolve alerts: refresh the whole module.
  const refresh = () => queryClient.invalidateQueries({ queryKey: alertsQueryKey })

  /** "new", the id of the rule being edited, or null when the form is closed. */
  const [editing, setEditing] = useState<string | null>(null)
  const form = useForm<AlertRuleValues>({
    resolver: zodResolver(alertRuleSchema),
    defaultValues: emptyRuleValues,
  })
  const type = useWatch({ control: form.control, name: 'type' })
  const metric = useWatch({ control: form.control, name: 'metric' })

  const save = useMutation({
    mutationFn: ({ id, input }: { id: string | null; input: AlertRuleInput }) =>
      id ? updateAlertRule(id, input) : createAlertRule(input),
    onSuccess: () => setEditing(null),
    onSettled: refresh,
  })
  const toggle = useMutation({
    mutationFn: ({ id, input }: { id: string; input: AlertRuleInput }) =>
      updateAlertRule(id, input),
    onSettled: refresh,
  })
  const [pendingDeletion, setPendingDeletion] = useState<RuleRow | null>(null)
  const remove = useMutation({
    mutationFn: (id: string) => deleteAlertRule(id),
    onSuccess: () => setPendingDeletion(null),
    onSettled: refresh,
  })

  const rules = useMemo(() => rulesQuery.data?.rules ?? [], [rulesQuery.data])
  const rows = useMemo(() => rules.map(toRuleRow), [rules])
  const catalog = rulesQuery.data?.catalog
  const metricCatalog =
    type === 'container_metric' ? catalog?.containerMetrics : catalog?.hostMetrics

  // A change of type or metric keeps the metric and the label valid.
  const fitMetric = () => {
    const fitted = fitMetricValues(form.getValues(), catalog)
    if (fitted.metric !== form.getValues('metric')) form.setValue('metric', fitted.metric)
    if (fitted.labelKey !== form.getValues('labelKey')) form.setValue('labelKey', fitted.labelKey)
  }

  const open = (id: string, values: AlertRuleValues) => {
    save.reset()
    form.reset(values)
    setEditing(id)
  }

  return {
    state: loadState(rulesQuery),
    rows,
    canManage,
    editing,
    isNew: editing === 'new',
    startCreate: () => open('new', emptyRuleValues),
    startEdit: (id: string) => {
      const rule = rules.find((candidate) => candidate.id === id)
      if (rule) open(id, ruleToValues(rule))
    },
    cancelEdit: () => setEditing(null),
    register: form.register,
    registerType: form.register('type', { onChange: fitMetric }),
    registerMetric: form.register('metric', { onChange: fitMetric }),
    errors: form.formState.errors,
    onSave: form.handleSubmit((values) =>
      save.mutate({ id: editing === 'new' ? null : editing, input: alertRuleInput(values) }),
    ),
    isSaving: save.isPending,
    saveError: save.error?.message ?? null,
    type,
    options: {
      types: ruleTypeOptions,
      severities: severityOptions,
      operators: operatorOptions,
      metrics: metricOptions(metricCatalog),
      labels: labelOptions(metricCatalog, metric),
      states: (catalog?.containerStates ?? []).map((value) => ({
        value,
        label: containerStateLabel(value),
      })),
      collectors: (catalog?.collectors ?? []).map((value) => ({
        value,
        label: collectorLabel(value),
      })),
      containers: [
        { value: '', label: 'Tous les conteneurs' },
        ...(containersQuery.data ?? []).map((container) => ({
          value: container.id,
          label: container.name,
        })),
      ],
    },
    toggleEnabled: (id: string) => {
      const rule = rules.find((candidate) => candidate.id === id)
      if (rule) toggle.mutate({ id, input: toggledRuleInput(rule) })
    },
    isToggling: toggle.isPending,
    actionError: toggle.error?.message ?? remove.error?.message ?? null,
    pendingDeletion,
    askDelete: (row: RuleRow) => {
      remove.reset()
      setPendingDeletion(row)
    },
    cancelDelete: () => setPendingDeletion(null),
    confirmDelete: () => {
      if (pendingDeletion) remove.mutate(pendingDeletion.id)
    },
    isDeleting: remove.isPending,
  }
}

export type AlertRulesPanelModel = ReturnType<typeof useAlertRulesPanel>
