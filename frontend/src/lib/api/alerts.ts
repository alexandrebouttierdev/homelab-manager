import { requestJSON } from './client'
import type { VulnSeverity } from './vulnerabilities'

/** Alerts and alert rules (API_CONTRACTS.md § 4.8). */
export type AlertSeverity = 'info' | 'warning' | 'critical'
export type AlertState = 'firing' | 'acknowledged' | 'resolved'
export type AlertRuleType =
  | 'host_metric'
  | 'container_metric'
  | 'container_state'
  | 'unit_failed'
  | 'collector'
  | 'vulnerabilities'
  | 'backup'
export type AlertOperator = '>' | '>=' | '<' | '<='
export type AlertResolution =
  | 'condition_cleared'
  | 'no_data'
  | 'target_gone'
  | 'rule_disabled'
  | 'rule_deleted'
  | 'rule_changed'

export type AlertTarget = {
  kind: 'host' | 'container' | 'unit' | 'collector' | 'vulnerabilities' | 'backup'
  metric: string | null
  labels: Record<string, string>
  containerId: string | null
  containerName: string | null
  unit: string | null
  collector: string | null
}

export type Alert = {
  id: string
  ruleId: string
  ruleName: string
  ruleType: AlertRuleType
  severity: AlertSeverity
  state: AlertState
  /** Stable key of the target, used by a target silence. */
  targetKey: string
  target: AlertTarget
  value: number | null
  threshold: number | null
  observed: string | null
  firedAt: string
  acknowledgedAt: string | null
  acknowledgedBy: string | null
  resolvedAt: string | null
  resolution: AlertResolution | null
  /** A silence in force covers the open alert: it does not notify. */
  silenced: boolean
  lastNotifiedAt: string | null
}

export type AlertSummary = {
  open: Record<AlertSeverity, number>
  resolvedLast24h: number
  meanResolutionSeconds24h: number | null
}

/** Parameters of a rule: each type uses its own (§ 4.8). */
export type AlertRuleParams = {
  metric?: string
  labels?: Record<string, string>
  operator?: AlertOperator
  threshold?: number
  containerId?: string
  states?: string[]
  unitPattern?: string
  collector?: string
  /** Vulnerabilities rule: minimum severity and fixable findings only. */
  vulnSeverity?: VulnSeverity
  fixableOnly?: boolean
  /** backup rule: age of the last successful backup that fires it, 1 to 720 hours. */
  maxAgeHours?: number
  forSeconds: number
}

export type AlertRule = {
  id: string
  name: string
  type: AlertRuleType
  severity: AlertSeverity
  enabled: boolean
  default: boolean
  params: AlertRuleParams
  createdAt: string
  updatedAt: string
}

/** Values the server accepts in a rule, with the labels of each metric. */
export type AlertCatalog = {
  hostMetrics: Record<string, string[]>
  containerMetrics: Record<string, string[]>
  containerStates: string[]
  collectors: string[]
  vulnerabilitySeverities: VulnSeverity[]
}

export type AlertRuleInput = {
  name: string
  type: AlertRuleType
  severity: AlertSeverity
  enabled: boolean
  params: AlertRuleParams
}

export type AlertListState = 'open' | 'resolved' | 'all'

export async function listAlerts(state: AlertListState, limit = 200): Promise<Alert[]> {
  const query = new URLSearchParams({ state, limit: String(limit) })
  const body = await requestJSON<{ alerts: Alert[] }>(`/api/v1/alerts?${query.toString()}`)
  return body.alerts
}

export function getAlertSummary(): Promise<AlertSummary> {
  return requestJSON<AlertSummary>('/api/v1/alerts/summary')
}

export async function acknowledgeAlert(id: string): Promise<Alert> {
  const body = await requestJSON<{ alert: Alert }>(
    `/api/v1/alerts/${encodeURIComponent(id)}/acknowledge`,
    { method: 'POST' },
  )
  return body.alert
}

export function listAlertRules(): Promise<{ rules: AlertRule[]; catalog: AlertCatalog }> {
  return requestJSON<{ rules: AlertRule[]; catalog: AlertCatalog }>('/api/v1/alert-rules')
}

export async function createAlertRule(input: AlertRuleInput): Promise<AlertRule> {
  const body = await requestJSON<{ rule: AlertRule }>('/api/v1/alert-rules', {
    method: 'POST',
    body: JSON.stringify(input),
  })
  return body.rule
}

export async function updateAlertRule(id: string, input: AlertRuleInput): Promise<AlertRule> {
  const body = await requestJSON<{ rule: AlertRule }>(
    `/api/v1/alert-rules/${encodeURIComponent(id)}`,
    { method: 'PUT', body: JSON.stringify(input) },
  )
  return body.rule
}

export async function deleteAlertRule(id: string): Promise<void> {
  await requestJSON<{ deleted: boolean }>(`/api/v1/alert-rules/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}
