import type { BadgeTone } from '../components/ui/Badge'
import type {
  Alert,
  AlertCatalog,
  AlertOperator,
  AlertResolution,
  AlertRule,
  AlertRuleInput,
  AlertRuleType,
  AlertSeverity,
  AlertState,
} from './api/alerts'
import { formatMetricValue } from './format'
import { vulnSeverityLabel } from './vulnerabilities'
import { parseDecimal, type AlertRuleValues } from '../schemas/alertRule'

const severityLabels: Record<AlertSeverity, string> = {
  info: 'Information',
  warning: 'Avertissement',
  critical: 'Critique',
}

export const severityOptions: ReadonlyArray<{ value: AlertSeverity; label: string }> = (
  ['info', 'warning', 'critical'] as const
).map((value) => ({ value, label: severityLabels[value] }))

export function severityLabel(severity: AlertSeverity): string {
  return severityLabels[severity] ?? severity
}

export function severityTone(severity: AlertSeverity): BadgeTone {
  return { info: 'info', warning: 'warning', critical: 'danger' }[severity] as BadgeTone
}

export function alertStateLabel(state: AlertState): string {
  return { firing: 'En cours', acknowledged: 'Acquittée', resolved: 'Résolue' }[state] ?? state
}

export function alertStateTone(state: AlertState): BadgeTone {
  return ({ firing: 'danger', acknowledged: 'warning', resolved: 'success' }[state] ??
    'neutral') as BadgeTone
}

const resolutionLabels: Record<AlertResolution, string> = {
  condition_cleared: 'Condition levée',
  no_data: 'Données absentes depuis 5 min',
  target_gone: 'Cible disparue',
  rule_disabled: 'Règle désactivée',
  rule_deleted: 'Règle supprimée',
  rule_changed: 'Règle modifiée',
}

export function resolutionLabel(resolution: AlertResolution | null): string {
  return resolution ? (resolutionLabels[resolution] ?? resolution) : '—'
}

const ruleTypeLabels: Record<AlertRuleType, string> = {
  host_metric: 'Métrique de la machine',
  container_metric: 'Métrique de conteneur',
  container_state: 'État de conteneur',
  unit_failed: 'Unité systemd en échec',
  collector: 'Collecteur indisponible',
  vulnerabilities: 'Vulnérabilités des paquets',
}

export const ruleTypeOptions: ReadonlyArray<{ value: AlertRuleType; label: string }> = (
  Object.keys(ruleTypeLabels) as AlertRuleType[]
).map((value) => ({ value, label: ruleTypeLabels[value] }))

export function ruleTypeLabel(type: AlertRuleType): string {
  return ruleTypeLabels[type] ?? type
}

export const operatorOptions: ReadonlyArray<{ value: AlertOperator; label: string }> = [
  { value: '>', label: '>' },
  { value: '>=', label: '≥' },
  { value: '<', label: '<' },
  { value: '<=', label: '≤' },
]

function operatorSymbol(operator: string | undefined): string {
  return operatorOptions.find((option) => option.value === operator)?.label ?? operator ?? ''
}

/** French names and units of the metrics a rule can watch (API_CONTRACTS.md § 5.4, § 5.5). */
const metrics: Record<string, { label: string; unit: string }> = {
  'cpu.percent': { label: 'Utilisation CPU', unit: 'percent' },
  'cpu.load1': { label: 'Charge 1 min', unit: 'processes' },
  'cpu.load5': { label: 'Charge 5 min', unit: 'processes' },
  'cpu.load15': { label: 'Charge 15 min', unit: 'processes' },
  'memory.used': { label: 'Mémoire utilisée', unit: 'bytes' },
  'memory.available': { label: 'Mémoire disponible', unit: 'bytes' },
  'memory.cached': { label: 'Mémoire en cache', unit: 'bytes' },
  'memory.available_percent': { label: 'Mémoire disponible (%)', unit: 'percent' },
  'memory.usage_bytes': { label: 'Mémoire utilisée', unit: 'bytes' },
  'memory.percent': { label: 'Mémoire (% de la limite)', unit: 'percent' },
  'memory.available_bytes': { label: 'Mémoire restante sous la limite', unit: 'bytes' },
  'swap.used': { label: 'Swap utilisé', unit: 'bytes' },
  'filesystem.used': { label: 'Espace disque utilisé', unit: 'bytes' },
  'filesystem.available': { label: 'Espace disque libre', unit: 'bytes' },
  'filesystem.percent': { label: 'Espace disque utilisé (%)', unit: 'percent' },
  'disk.read_iops': { label: 'Lectures disque', unit: 'iops' },
  'disk.write_iops': { label: 'Écritures disque', unit: 'iops' },
  'disk.read_latency_ms': { label: 'Latence de lecture', unit: 'milliseconds' },
  'disk.write_latency_ms': { label: "Latence d'écriture", unit: 'milliseconds' },
  'network.rx_bytes_per_second': { label: 'Débit entrant', unit: 'bytes_per_second' },
  'network.tx_bytes_per_second': { label: 'Débit sortant', unit: 'bytes_per_second' },
  'network.errors': { label: 'Erreurs réseau', unit: 'count' },
  'network.drops': { label: 'Paquets rejetés', unit: 'count' },
  'temperature.celsius': { label: 'Température', unit: 'celsius' },
  'process.count': { label: 'Processus', unit: 'count' },
  'restart.count': { label: 'Redémarrages', unit: 'count' },
}

export function metricLabel(name: string): string {
  return metrics[name]?.label ?? name
}

function metricUnit(name: string | null | undefined): string | null {
  return name ? (metrics[name]?.unit ?? null) : null
}

const labelNames: Record<string, string> = {
  mountpoint: 'Point de montage',
  device: 'Périphérique',
  interface: 'Interface',
  sensor: 'Capteur',
  core: 'Cœur',
}

export function labelName(key: string): string {
  return labelNames[key] ?? key
}

const collectorLabels: Record<string, string> = {
  system: 'Collecte système',
  docker: 'Collecte Docker',
  services: 'Services système',
}

export function collectorLabel(name: string): string {
  return collectorLabels[name] ?? name
}

const containerStateLabels: Record<string, string> = {
  created: 'Créé',
  running: 'En cours',
  paused: 'En pause',
  restarting: 'Redémarrage',
  removing: 'Suppression',
  exited: 'Arrêté',
  dead: 'Mort',
}

export function containerStateLabel(state: string): string {
  return containerStateLabels[state] ?? state
}

/** A duration in seconds: "immédiat", "45 s", "5 min", "1 h 30 min". */
export function formatAlertDuration(seconds: number | null | undefined): string {
  if (seconds == null || !Number.isFinite(seconds)) return '—'
  if (seconds <= 0) return 'immédiat'
  const total = Math.round(seconds)
  if (total < 60) return `${total} s`
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const rest = total % 60
  if (hours > 0) return minutes > 0 ? `${hours} h ${minutes} min` : `${hours} h`
  return rest > 0 ? `${minutes} min ${rest} s` : `${minutes} min`
}

function labelsText(labels: Record<string, string> | undefined): string {
  const entries = Object.entries(labels ?? {}).sort(([a], [b]) => a.localeCompare(b))
  return entries.map(([key, value]) => `${labelName(key)} ${value}`).join(', ')
}

/** One line describing what a rule watches. */
export function describeRule(rule: Pick<AlertRule, 'type' | 'params'>): string {
  const p = rule.params
  const during = p.forSeconds > 0 ? ` pendant ${formatAlertDuration(p.forSeconds)}` : ''
  switch (rule.type) {
    case 'host_metric':
    case 'container_metric': {
      const threshold = formatMetricValue(p.threshold ?? null, metricUnit(p.metric))
      const labels = labelsText(p.labels)
      const scope =
        rule.type === 'container_metric'
          ? p.containerId
            ? ' (un conteneur)'
            : ' (tous les conteneurs)'
          : ''
      return `${metricLabel(p.metric ?? '')}${labels ? ` [${labels}]` : ''}${scope} ${operatorSymbol(p.operator)} ${threshold}${during}`
    }
    case 'container_state': {
      const states = (p.states ?? []).map(containerStateLabel).join(' ou ')
      const scope = p.containerId ? 'Le conteneur' : 'Un conteneur'
      return `${scope} est ${states.toLowerCase()}${during}`
    }
    case 'unit_failed':
      return p.unitPattern === '*'
        ? 'Une unité systemd est en échec'
        : `Une unité correspondant à « ${p.unitPattern ?? ''} » est en échec`
    case 'collector':
      return `${collectorLabel(p.collector ?? '')} indisponible${during}`
    case 'vulnerabilities': {
      const severity = vulnSeverityLabel(p.vulnSeverity ?? 'critical').toLowerCase()
      return `Une vulnérabilité ${severity} ou plus grave${p.fixableOnly ? ', corrigeable,' : ''} est présente`
    }
  }
}

/** The target of an alert, for display. */
export function describeTarget(alert: Alert): string {
  const target = alert.target
  switch (target.kind) {
    case 'host': {
      const labels = labelsText(target.labels)
      return labels ? `Machine · ${labels}` : 'Machine'
    }
    case 'container': {
      const labels = labelsText(target.labels)
      const name = target.containerName ?? target.containerId ?? 'Conteneur'
      return labels ? `${name} · ${labels}` : name
    }
    case 'unit':
      return target.unit ?? 'Unité'
    case 'collector':
      return collectorLabel(target.collector ?? '')
    case 'vulnerabilities':
      return 'Paquets de la machine'
  }
}

/** The measured value and the threshold, or the state that fired the rule. */
export function alertValueText(alert: Alert): string {
  if (alert.target.kind === 'vulnerabilities' && alert.value !== null)
    return `${alert.value} vulnérabilité${alert.value > 1 ? 's' : ''}`
  if (alert.value !== null) {
    const unit = metricUnit(alert.target.metric)
    const value = formatMetricValue(alert.value, unit)
    if (alert.threshold === null) return value
    return `${value} (seuil ${formatMetricValue(alert.threshold, unit)})`
  }
  if (!alert.observed) return '—'
  if (alert.ruleType === 'container_state') return containerStateLabel(alert.observed)
  return alert.observed
}

export type AlertLink = { to: string; label: string }

/** Where to look for the cause (SPEC_ADMINISTRATION.md § 6.4). */
export function alertLink(alert: Alert): AlertLink | null {
  const target = alert.target
  switch (target.kind) {
    case 'unit':
      return target.unit
        ? {
            to: `/services?${new URLSearchParams({ unit: target.unit }).toString()}`,
            label: "Voir l'unité et son journal",
          }
        : null
    case 'container':
      return target.containerId
        ? {
            to: `/containers/${encodeURIComponent(target.containerId)}`,
            label: 'Voir le conteneur',
          }
        : null
    case 'host':
      return { to: '/metrics', label: 'Voir les métriques' }
    case 'collector':
      return target.collector === 'services'
        ? { to: '/services', label: 'Voir les services' }
        : { to: '/', label: 'Voir la synthèse' }
    case 'vulnerabilities':
      return { to: '/host?tab=vulnerabilities', label: 'Voir les vulnérabilités' }
  }
}

const severityRank: Record<AlertSeverity, number> = { critical: 0, warning: 1, info: 2 }

/** Open alerts, most severe first, then most recent. */
export function sortOpenAlerts(alerts: Alert[]): Alert[] {
  return [...alerts].sort(
    (a, b) =>
      severityRank[a.severity] - severityRank[b.severity] || b.firedAt.localeCompare(a.firedAt),
  )
}

/** Duration of an alert, until now when it is still open. */
export function alertDurationSeconds(alert: Alert, now: Date): number {
  const end = alert.resolvedAt ? Date.parse(alert.resolvedAt) : now.getTime()
  return Math.max(0, (end - Date.parse(alert.firedAt)) / 1000)
}

export const emptyRuleValues: AlertRuleValues = {
  name: '',
  type: 'host_metric',
  severity: 'warning',
  enabled: true,
  metric: 'filesystem.percent',
  labelKey: '',
  labelValue: '',
  operator: '>',
  threshold: '85',
  containerId: '',
  states: ['exited'],
  unitPattern: '*',
  collector: 'docker',
  forMinutes: '5',
  vulnSeverity: 'critical',
  fixableOnly: false,
}

/** Form values of an existing rule. The form edits one label; a rule from the API keeps its first one. */
export function ruleToValues(rule: AlertRule): AlertRuleValues {
  const p = rule.params
  const [labelKey, labelValue] = Object.entries(p.labels ?? {})[0] ?? ['', '']
  return {
    ...emptyRuleValues,
    name: rule.name,
    type: rule.type,
    severity: rule.severity,
    enabled: rule.enabled,
    metric: p.metric ?? emptyRuleValues.metric,
    labelKey,
    labelValue,
    operator: p.operator ?? '>',
    threshold: p.threshold === undefined ? '' : String(p.threshold),
    containerId: p.containerId ?? '',
    states: p.states ?? [],
    unitPattern: p.unitPattern ?? '*',
    collector: p.collector ?? 'docker',
    forMinutes: String(p.forSeconds / 60),
    vulnSeverity:
      p.vulnSeverity === 'negligible' || p.vulnSeverity === 'unknown' || !p.vulnSeverity
        ? 'critical'
        : p.vulnSeverity,
    fixableOnly: p.fixableOnly ?? false,
  }
}

/** The API body of validated form values: only the parameters of the type. */
export function alertRuleInput(values: AlertRuleValues): AlertRuleInput {
  const forSeconds = Math.round((parseDecimal(values.forMinutes) ?? 0) * 60)
  const common = {
    name: values.name.trim(),
    type: values.type,
    severity: values.severity,
    enabled: values.enabled,
  }
  switch (values.type) {
    case 'host_metric':
    case 'container_metric': {
      const params: AlertRuleInput['params'] = {
        metric: values.metric,
        operator: values.operator,
        threshold: parseDecimal(values.threshold) ?? 0,
        forSeconds,
      }
      if (values.labelKey && values.labelValue.trim())
        params.labels = { [values.labelKey]: values.labelValue.trim() }
      if (values.type === 'container_metric' && values.containerId)
        params.containerId = values.containerId
      return { ...common, params }
    }
    case 'container_state': {
      const params: AlertRuleInput['params'] = { states: values.states, forSeconds }
      if (values.containerId) params.containerId = values.containerId
      return { ...common, params }
    }
    case 'unit_failed':
      return { ...common, params: { unitPattern: values.unitPattern.trim(), forSeconds: 0 } }
    case 'collector':
      return { ...common, params: { collector: values.collector, forSeconds } }
    case 'vulnerabilities': {
      const params: AlertRuleInput['params'] = { vulnSeverity: values.vulnSeverity, forSeconds: 0 }
      if (values.fixableOnly) params.fixableOnly = true
      return { ...common, params }
    }
  }
}

export type AlertRow = {
  id: string
  ruleName: string
  severity: { label: string; tone: BadgeTone }
  state: { label: string; tone: BadgeTone }
  target: string
  value: string
  firedAt: string
  duration: string
  acknowledgedBy: string | null
  resolution: string
  link: AlertLink | null
  /** Only a firing alert can be acknowledged. */
  acknowledgeable: boolean
  /** An active silence covers the open alert: it does not notify. */
  silenced: boolean
  /** An open alert that no silence covers yet can be silenced. */
  silenceable: boolean
  /** Unit an open unit_failed alert offers to restart: in the allow list only (§ 6.4). */
  restartUnit: string | null
}

/** The unit of an open alert that can be restarted from the alert (SPEC_ADMINISTRATION.md § 6.4). */
export function alertRestartUnit(alert: Alert, manageable: string[]): string | null {
  const unit = alert.target.kind === 'unit' ? alert.target.unit : null
  if (!unit || alert.state === 'resolved' || !manageable.includes(unit)) return null
  return unit
}

/** manageable is the allow list of the service actions, [] when the user cannot act. */
export function toAlertRow(alert: Alert, now: Date, manageable: string[] = []): AlertRow {
  return {
    id: alert.id,
    ruleName: alert.ruleName,
    severity: { label: severityLabel(alert.severity), tone: severityTone(alert.severity) },
    state: { label: alertStateLabel(alert.state), tone: alertStateTone(alert.state) },
    target: describeTarget(alert),
    value: alertValueText(alert),
    firedAt: alert.firedAt,
    duration: formatAlertDuration(alertDurationSeconds(alert, now)),
    acknowledgedBy: alert.acknowledgedBy,
    resolution: resolutionLabel(alert.resolution),
    link: alertLink(alert),
    acknowledgeable: alert.state === 'firing',
    silenced: alert.silenced,
    silenceable: alert.state !== 'resolved' && !alert.silenced,
    restartUnit: alertRestartUnit(alert, manageable),
  }
}

export type RuleRow = {
  id: string
  name: string
  type: string
  description: string
  severity: { label: string; tone: BadgeTone }
  enabled: boolean
  isDefault: boolean
}

export function toRuleRow(rule: AlertRule): RuleRow {
  return {
    id: rule.id,
    name: rule.name,
    type: ruleTypeLabel(rule.type),
    description: describeRule(rule),
    severity: { label: severityLabel(rule.severity), tone: severityTone(rule.severity) },
    enabled: rule.enabled,
    isDefault: rule.default,
  }
}

/** The body that switches a rule on or off, parameters unchanged. */
export function toggledRuleInput(rule: AlertRule): AlertRuleInput {
  return {
    name: rule.name,
    type: rule.type,
    severity: rule.severity,
    enabled: !rule.enabled,
    params: rule.params,
  }
}

/** Metric options of a rule type, sorted by their French name. */
export function metricOptions(
  catalog: Record<string, string[]> | undefined,
): Array<{ value: string; label: string }> {
  return Object.keys(catalog ?? {})
    .map((value) => ({ value, label: metricLabel(value) }))
    .sort((a, b) => a.label.localeCompare(b.label, 'fr'))
}

/** Label options of a metric: "none" then its labels. */
export function labelOptions(
  catalog: Record<string, string[]> | undefined,
  metric: string,
): Array<{ value: string; label: string }> {
  return [
    { value: '', label: 'Toutes les séries' },
    ...(catalog?.[metric] ?? []).map((key) => ({ value: key, label: labelName(key) })),
  ]
}

/** Mean resolution time of the summary, or a dash without resolved alerts. */
export function formatMeanResolution(seconds: number | null): string {
  return seconds === null ? '—' : formatAlertDuration(seconds)
}

/**
 * Metric and label kept valid after the type or the metric changed: a host
 * metric may not exist on a container, a label not on another metric.
 */
export function fitMetricValues(
  values: Pick<AlertRuleValues, 'type' | 'metric' | 'labelKey'>,
  catalog: AlertCatalog | undefined,
): { metric: string; labelKey: string } {
  const metrics =
    values.type === 'container_metric' ? catalog?.containerMetrics : catalog?.hostMetrics
  if (!metrics) return { metric: values.metric, labelKey: values.labelKey }
  const metric = values.metric in metrics ? values.metric : (metricOptions(metrics)[0]?.value ?? '')
  const labelKey = (metrics[metric] ?? []).includes(values.labelKey) ? values.labelKey : ''
  return { metric, labelKey }
}
