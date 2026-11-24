import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  acknowledgeAlert,
  createAlertRule,
  deleteAlertRule,
  getAlertSummary,
  listAlertRules,
  listAlerts,
  updateAlertRule,
  type AlertRuleInput,
} from './alerts'

function mockFetch(body: unknown, status = 200) {
  const fetchMock = vi
    .fn()
    .mockImplementation(() => Promise.resolve(new Response(JSON.stringify(body), { status })))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

const input: AlertRuleInput = {
  name: 'Disque',
  type: 'host_metric',
  severity: 'warning',
  enabled: true,
  params: { metric: 'filesystem.percent', operator: '>', threshold: 85, forSeconds: 300 },
}

describe('alerts API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('lists alerts by state', async () => {
    const fetchMock = mockFetch({ alerts: [] })
    await expect(listAlerts('open')).resolves.toEqual([])
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/alerts?state=open&limit=200')
  })

  it('reads the summary', async () => {
    const summary = {
      open: { info: 0, warning: 1, critical: 0 },
      resolvedLast24h: 0,
      meanResolutionSeconds24h: null,
    }
    mockFetch(summary)
    await expect(getAlertSummary()).resolves.toEqual(summary)
  })

  it('acknowledges an alert with a POST', async () => {
    const fetchMock = mockFetch({ alert: { id: 'a1' } })
    await expect(acknowledgeAlert('a1')).resolves.toEqual({ id: 'a1' })
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/alerts/a1/acknowledge')
    expect(fetchMock.mock.calls[0][1].method).toBe('POST')
  })

  it('creates, replaces and deletes rules', async () => {
    const fetchMock = mockFetch({ rule: { id: 'r1' } })
    await createAlertRule(input)
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/alert-rules')
    expect(JSON.parse(fetchMock.mock.calls[0][1].body as string)).toEqual(input)
    await updateAlertRule('r1', input)
    expect(fetchMock.mock.calls[1][0]).toBe('/api/v1/alert-rules/r1')
    expect(fetchMock.mock.calls[1][1].method).toBe('PUT')
    mockFetch({ deleted: true })
    await expect(deleteAlertRule('r1')).resolves.toBeUndefined()
  })

  it('reads the rules with their catalog', async () => {
    const body = {
      rules: [],
      catalog: { hostMetrics: {}, containerMetrics: {}, containerStates: [], collectors: [] },
    }
    mockFetch(body)
    await expect(listAlertRules()).resolves.toEqual(body)
  })

  it('explains a deleted default rule', async () => {
    mockFetch({ error: { code: 'DEFAULT_RULE', message: 'default' } }, 409)
    await expect(deleteAlertRule('r1')).rejects.toThrow(/par défaut/)
  })
})
