import { describe, expect, it } from 'vitest'
import type { Alert, AlertRule } from './api/alerts'
import {
  alertDurationSeconds,
  alertLink,
  alertRestartUnit,
  alertRuleInput,
  alertValueText,
  describeRule,
  describeTarget,
  emptyRuleValues,
  fitMetricValues,
  formatAlertDuration,
  formatMeanResolution,
  labelOptions,
  metricOptions,
  resolutionLabel,
  ruleToValues,
  severityTone,
  sortOpenAlerts,
  toAlertRow,
  toggledRuleInput,
  toRuleRow,
} from './alerts'

function alert(overrides: Partial<Alert> = {}): Alert {
  return {
    id: 'a1',
    ruleId: 'r1',
    ruleName: 'Disque presque plein',
    ruleType: 'host_metric',
    severity: 'warning',
    state: 'firing',
    target: {
      kind: 'host',
      metric: 'filesystem.percent',
      labels: { mountpoint: '/', device: 'sda1' },
      containerId: null,
      containerName: null,
      unit: null,
      collector: null,
    },
    value: 91.5,
    threshold: 85,
    observed: null,
    firedAt: '2026-11-24T10:00:00Z',
    acknowledgedAt: null,
    acknowledgedBy: null,
    resolvedAt: null,
    resolution: null,
    targetKey: 'host:x',
    silenced: false,
    lastNotifiedAt: null,
    ...overrides,
  }
}

function rule(overrides: Partial<AlertRule> = {}): AlertRule {
  return {
    id: 'r1',
    name: 'Disque',
    type: 'host_metric',
    severity: 'warning',
    enabled: true,
    default: false,
    params: { metric: 'filesystem.percent', operator: '>', threshold: 85, forSeconds: 300 },
    createdAt: '2026-11-24T00:00:00Z',
    updatedAt: '2026-11-24T00:00:00Z',
    ...overrides,
  }
}

describe('alert labels', () => {
  it('formats durations', () => {
    expect(formatAlertDuration(0)).toBe('immédiat')
    expect(formatAlertDuration(45)).toBe('45 s')
    expect(formatAlertDuration(300)).toBe('5 min')
    expect(formatAlertDuration(90)).toBe('1 min 30 s')
    expect(formatAlertDuration(5400)).toBe('1 h 30 min')
    expect(formatAlertDuration(7200)).toBe('2 h')
    expect(formatAlertDuration(null)).toBe('—')
  })

  it('tones severities and names resolutions', () => {
    expect(severityTone('critical')).toBe('danger')
    expect(severityTone('info')).toBe('info')
    expect(resolutionLabel('no_data')).toBe('Données absentes depuis 5 min')
    expect(resolutionLabel(null)).toBe('—')
  })

  it('describes each rule type', () => {
    expect(describeRule(rule())).toBe('Espace disque utilisé (%) > 85 % pendant 5 min')
    expect(
      describeRule(
        rule({
          params: {
            metric: 'filesystem.percent',
            labels: { mountpoint: '/var/lib/docker' },
            operator: '>=',
            threshold: 90,
            forSeconds: 0,
          },
        }),
      ),
    ).toBe('Espace disque utilisé (%) [Point de montage /var/lib/docker] ≥ 90 %')
    expect(
      describeRule(
        rule({
          type: 'container_metric',
          params: { metric: 'memory.percent', operator: '>', threshold: 90, forSeconds: 60 },
        }),
      ),
    ).toBe('Mémoire (% de la limite) (tous les conteneurs) > 90 % pendant 1 min')
    expect(
      describeRule(
        rule({ type: 'container_state', params: { states: ['exited', 'dead'], forSeconds: 120 } }),
      ),
    ).toBe('Un conteneur est arrêté ou mort pendant 2 min')
    expect(
      describeRule(rule({ type: 'unit_failed', params: { unitPattern: '*', forSeconds: 0 } })),
    ).toBe('Une unité systemd est en échec')
    expect(
      describeRule(
        rule({ type: 'unit_failed', params: { unitPattern: 'backup-*', forSeconds: 0 } }),
      ),
    ).toBe('Une unité correspondant à « backup-* » est en échec')
    expect(
      describeRule(rule({ type: 'collector', params: { collector: 'docker', forSeconds: 120 } })),
    ).toBe('Collecte Docker indisponible pendant 2 min')
  })
})

describe('alert targets and values', () => {
  it('describes targets', () => {
    expect(describeTarget(alert())).toBe('Machine · Périphérique sda1, Point de montage /')
    expect(
      describeTarget(
        alert({
          target: {
            ...alert().target,
            kind: 'container',
            labels: {},
            containerId: 'c1',
            containerName: 'web',
          },
        }),
      ),
    ).toBe('web')
    expect(
      describeTarget(
        alert({ target: { ...alert().target, kind: 'unit', labels: {}, unit: 'ssh.service' } }),
      ),
    ).toBe('ssh.service')
    expect(
      describeTarget(
        alert({
          target: { ...alert().target, kind: 'collector', labels: {}, collector: 'docker' },
        }),
      ),
    ).toBe('Collecte Docker')
  })

  it('shows the value with the threshold, or the observed state', () => {
    expect(alertValueText(alert())).toBe('91,5 % (seuil 85 %)')
    expect(
      alertValueText(
        alert({ ruleType: 'container_state', value: null, threshold: null, observed: 'exited' }),
      ),
    ).toBe('Arrêté')
    expect(
      alertValueText(
        alert({
          ruleType: 'unit_failed',
          value: null,
          threshold: null,
          observed: 'failed (exit-code)',
        }),
      ),
    ).toBe('failed (exit-code)')
    expect(alertValueText(alert({ value: null, threshold: null, observed: null }))).toBe('—')
  })

  it('links to the cause', () => {
    expect(
      alertLink(alert({ target: { ...alert().target, kind: 'unit', unit: 'getty@tty1.service' } })),
    ).toEqual({
      to: '/services?unit=getty%40tty1.service',
      label: "Voir l'unité et son journal",
    })
    expect(
      alertLink(alert({ target: { ...alert().target, kind: 'container', containerId: 'c1' } }))?.to,
    ).toBe('/containers/c1')
    expect(alertLink(alert())?.to).toBe('/metrics')
  })

  it('sorts open alerts by severity then recency', () => {
    const sorted = sortOpenAlerts([
      alert({ id: 'old-warning', firedAt: '2026-11-24T08:00:00Z' }),
      alert({ id: 'critical', severity: 'critical', firedAt: '2026-11-24T07:00:00Z' }),
      alert({ id: 'new-warning', firedAt: '2026-11-24T09:00:00Z' }),
      alert({ id: 'info', severity: 'info', firedAt: '2026-11-24T10:00:00Z' }),
    ])
    expect(sorted.map((a) => a.id)).toEqual(['critical', 'new-warning', 'old-warning', 'info'])
  })

  it('measures the duration until now or the resolution', () => {
    const now = new Date('2026-11-24T10:30:00Z')
    expect(alertDurationSeconds(alert(), now)).toBe(1800)
    expect(alertDurationSeconds(alert({ resolvedAt: '2026-11-24T10:05:00Z' }), now)).toBe(300)
  })
})

describe('rule form', () => {
  it('sends only the parameters of the type', () => {
    expect(
      alertRuleInput({
        ...emptyRuleValues,
        name: ' Disque docker ',
        labelKey: 'mountpoint',
        labelValue: '/var/lib/docker ',
        threshold: '85,5',
        forMinutes: '0,5',
      }),
    ).toEqual({
      name: 'Disque docker',
      type: 'host_metric',
      severity: 'warning',
      enabled: true,
      params: {
        metric: 'filesystem.percent',
        labels: { mountpoint: '/var/lib/docker' },
        operator: '>',
        threshold: 85.5,
        forSeconds: 30,
      },
    })
    expect(
      alertRuleInput({
        ...emptyRuleValues,
        name: 'Web',
        type: 'container_state',
        states: ['exited'],
        containerId: 'c1',
      }).params,
    ).toEqual({ states: ['exited'], containerId: 'c1', forSeconds: 300 })
    expect(
      alertRuleInput({
        ...emptyRuleValues,
        name: 'U',
        type: 'unit_failed',
        unitPattern: ' ssh.service ',
      }).params,
    ).toEqual({
      unitPattern: 'ssh.service',
      forSeconds: 0,
    })
    expect(
      alertRuleInput({ ...emptyRuleValues, name: 'C', type: 'collector', collector: 'system' })
        .params,
    ).toEqual({
      collector: 'system',
      forSeconds: 300,
    })
  })

  it('reads a rule back into the form', () => {
    const values = ruleToValues(
      rule({
        params: {
          metric: 'temperature.celsius',
          labels: { sensor: 'cpu_thermal' },
          operator: '>',
          threshold: 85,
          forSeconds: 120,
        },
      }),
    )
    expect(values).toMatchObject({
      metric: 'temperature.celsius',
      labelKey: 'sensor',
      labelValue: 'cpu_thermal',
      threshold: '85',
      forMinutes: '2',
    })
    expect(alertRuleInput(values).params).toEqual(
      rule({
        params: {
          metric: 'temperature.celsius',
          labels: { sensor: 'cpu_thermal' },
          operator: '>',
          threshold: 85,
          forSeconds: 120,
        },
      }).params,
    )
  })
})

describe('alert rows', () => {
  it('prepares an alert row', () => {
    const row = toAlertRow(alert(), new Date('2026-11-24T10:05:00Z'))
    expect(row).toMatchObject({
      severity: { label: 'Avertissement', tone: 'warning' },
      state: { label: 'En cours', tone: 'danger' },
      duration: '5 min',
      acknowledgeable: true,
      resolution: '—',
    })
    expect(toAlertRow(alert({ state: 'acknowledged' }), new Date()).acknowledgeable).toBe(false)
    expect(row.restartUnit).toBeNull()
  })

  it('offers to restart the unit of an open alert, in the allow list only', () => {
    const failed = alert({
      ruleType: 'unit_failed',
      target: { ...alert().target, kind: 'unit', metric: null, labels: {}, unit: 'backup.service' },
    })
    expect(alertRestartUnit(failed, ['backup.service'])).toBe('backup.service')
    expect(alertRestartUnit(failed, ['nginx.service'])).toBeNull()
    expect(alertRestartUnit(failed, [])).toBeNull()
    expect(alertRestartUnit({ ...failed, state: 'resolved' }, ['backup.service'])).toBeNull()
    expect(alertRestartUnit(alert(), ['backup.service'])).toBeNull()
    expect(toAlertRow(failed, new Date(), ['backup.service']).restartUnit).toBe('backup.service')
  })

  it('prepares a rule row and its toggle', () => {
    expect(toRuleRow(rule({ default: true }))).toMatchObject({
      type: 'Métrique de la machine',
      isDefault: true,
      severity: { tone: 'warning' },
    })
    expect(toggledRuleInput(rule())).toEqual({
      name: 'Disque',
      type: 'host_metric',
      severity: 'warning',
      enabled: false,
      params: rule().params,
    })
  })

  it('lists metric and label options', () => {
    const catalog = { 'filesystem.percent': ['mountpoint', 'device'], 'cpu.load1': [] }
    expect(metricOptions(catalog).map((option) => option.value)).toEqual([
      'cpu.load1',
      'filesystem.percent',
    ])
    expect(labelOptions(catalog, 'filesystem.percent').map((option) => option.label)).toEqual([
      'Toutes les séries',
      'Point de montage',
      'Périphérique',
    ])
    expect(labelOptions(catalog, 'cpu.load1')).toHaveLength(1)
    expect(formatMeanResolution(null)).toBe('—')
    expect(formatMeanResolution(840)).toBe('14 min')
  })
})

describe('metric fields', () => {
  const catalog = {
    hostMetrics: { 'filesystem.percent': ['mountpoint', 'device'], 'cpu.load1': [] },
    containerMetrics: { 'memory.percent': [], 'network.errors': ['interface'] },
    containerStates: [],
    collectors: [],
    vulnerabilitySeverities: [],
  }

  it('keeps a metric and a label that exist', () => {
    expect(
      fitMetricValues(
        { type: 'host_metric', metric: 'filesystem.percent', labelKey: 'device' },
        catalog,
      ),
    ).toEqual({ metric: 'filesystem.percent', labelKey: 'device' })
  })

  it('replaces a host metric on a container rule and drops a foreign label', () => {
    expect(
      fitMetricValues(
        { type: 'container_metric', metric: 'filesystem.percent', labelKey: 'mountpoint' },
        catalog,
      ),
    ).toEqual({ metric: 'network.errors', labelKey: '' })
  })

  it('leaves the values alone before the catalog is loaded', () => {
    expect(fitMetricValues({ type: 'host_metric', metric: 'x', labelKey: 'y' }, undefined)).toEqual(
      {
        metric: 'x',
        labelKey: 'y',
      },
    )
  })
})

describe('vulnerabilities rule', () => {
  it('describes, converts and links a vulnerabilities rule', () => {
    const rule = {
      type: 'vulnerabilities' as const,
      params: { vulnSeverity: 'high' as const, fixableOnly: true, forSeconds: 0 },
    }
    expect(describeRule(rule)).toBe(
      'Une vulnérabilité élevée ou plus grave, corrigeable, est présente',
    )
    const values = {
      ...emptyRuleValues,
      name: 'Vulnérabilités',
      type: 'vulnerabilities' as const,
      vulnSeverity: 'high' as const,
      fixableOnly: true,
    }
    expect(alertRuleInput(values).params).toEqual({
      vulnSeverity: 'high',
      fixableOnly: true,
      forSeconds: 0,
    })
    expect(alertRuleInput({ ...values, fixableOnly: false }).params).toEqual({
      vulnSeverity: 'high',
      forSeconds: 0,
    })
  })
})
