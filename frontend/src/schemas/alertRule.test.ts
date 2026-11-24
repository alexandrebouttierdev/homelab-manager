import { describe, expect, it } from 'vitest'
import { alertRuleSchema, parseDecimal, type AlertRuleValues } from './alertRule'

const base: AlertRuleValues = {
  name: 'Disque',
  type: 'host_metric',
  severity: 'warning',
  enabled: true,
  metric: 'filesystem.percent',
  labelKey: '',
  labelValue: '',
  operator: '>',
  threshold: '85',
  containerId: '',
  states: [],
  unitPattern: '',
  collector: '',
  forMinutes: '5',
}

function issues(values: Partial<AlertRuleValues>): string[] {
  const result = alertRuleSchema.safeParse({ ...base, ...values })
  return result.success ? [] : result.error.issues.map((issue) => issue.path.join('.'))
}

describe('alert rule schema', () => {
  it('accepts a metric rule with a decimal comma', () => {
    expect(issues({ threshold: '85,5', forMinutes: '0,5' })).toEqual([])
  })

  it('requires a name, a threshold and a label value', () => {
    expect(issues({ name: '  ' })).toEqual(['name'])
    expect(issues({ threshold: 'beaucoup' })).toEqual(['threshold'])
    expect(issues({ labelKey: 'mountpoint', labelValue: ' ' })).toEqual(['labelValue'])
  })

  it('bounds the duration', () => {
    expect(issues({ forMinutes: '-1' })).toEqual(['forMinutes'])
    expect(issues({ forMinutes: '1441' })).toEqual(['forMinutes'])
    expect(issues({ forMinutes: '' })).toEqual(['forMinutes'])
  })

  it('checks the parameters of each type only', () => {
    expect(issues({ type: 'container_state', threshold: '', states: [] })).toEqual(['states'])
    expect(issues({ type: 'unit_failed', unitPattern: '../x', forMinutes: '' })).toEqual([
      'unitPattern',
    ])
    expect(
      issues({ type: 'unit_failed', unitPattern: 'backup-*.service', forMinutes: '' }),
    ).toEqual([])
    expect(issues({ type: 'collector', collector: '', threshold: '' })).toEqual(['collector'])
  })

  it('parses decimals strictly', () => {
    expect(parseDecimal('12')).toBe(12)
    expect(parseDecimal(' 0,25 ')).toBe(0.25)
    expect(parseDecimal('-3.5')).toBe(-3.5)
    expect(parseDecimal('1e3')).toBeNull()
    expect(parseDecimal('')).toBeNull()
  })
})
