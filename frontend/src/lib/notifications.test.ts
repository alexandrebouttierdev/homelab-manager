import { describe, expect, it } from 'vitest'
import type { Alert } from './api/alerts'
import type { NotificationChannel, Silence } from './api/notifications'
import {
  channelInput,
  channelStatus,
  channelToValues,
  deliveryErrorText,
  emptyChannelValues,
  emptySilenceValues,
  formatRemaining,
  silenceInput,
  silenceScopeOptions,
  silenceSubject,
  toChannelRow,
  toSilenceRow,
  toggledChannelInput,
} from './notifications'

const now = new Date('2026-11-24T10:00:00Z')

const alert = {
  id: 'a',
  ruleId: 'r1',
  ruleName: 'Disque presque plein',
  targetKey: 'host:filesystem.percent{mountpoint=/}',
  target: {
    kind: 'host',
    metric: 'filesystem.percent',
    labels: { mountpoint: '/' },
    containerId: null,
    containerName: null,
    unit: null,
    collector: null,
  },
} as unknown as Alert

const silence: Silence = {
  id: 's',
  scope: 'target',
  ruleId: 'r1',
  ruleName: 'Disque presque plein',
  targetKey: 'host:filesystem.percent{mountpoint=/}',
  reason: 'Migration',
  createdBy: 'otto',
  startsAt: '2026-11-24T09:00:00Z',
  endsAt: '2026-11-24T12:05:00Z',
  liftedAt: null,
  liftedBy: null,
  active: true,
}

const channel: NotificationChannel = {
  id: 'c',
  name: 'Mail',
  type: 'smtp',
  enabled: true,
  minSeverity: 'warning',
  renotifyMinutes: 30,
  settings: {
    host: 'smtp.example.org',
    port: 587,
    security: 'starttls',
    username: 'bot',
    from: 'bot@example.org',
    to: ['a@example.org', 'b@example.org'],
  },
  secrets: { password: { defined: true, fingerprint: '3f9a0c1b7e42' } },
  lastSentAt: '2026-11-24T09:00:00Z',
  lastErrorAt: null,
  lastError: null,
  lastErrorStatus: null,
  createdAt: '2026-11-24T08:00:00Z',
  updatedAt: '2026-11-24T08:00:00Z',
}

describe('silences', () => {
  it('builds the request from an alert or for every alert', () => {
    const subject = silenceSubject(alert)
    expect(silenceScopeOptions(subject).map((option) => option.value)).toEqual(['target', 'rule'])
    expect(silenceScopeOptions(null).map((option) => option.value)).toEqual(['all'])
    expect(emptySilenceValues(null).scope).toBe('all')
    const values = {
      scope: 'target' as const,
      durationMinutes: '240' as const,
      reason: ' Migration ',
    }
    expect(silenceInput(values, subject)).toEqual({
      scope: 'target',
      ruleId: 'r1',
      targetKey: 'host:filesystem.percent{mountpoint=/}',
      durationMinutes: 240,
      reason: 'Migration',
    })
    expect(silenceInput({ ...values, scope: 'rule' }, subject)).toEqual({
      scope: 'rule',
      ruleId: 'r1',
      durationMinutes: 240,
      reason: 'Migration',
    })
    expect(silenceInput(values, null)).toEqual({
      scope: 'all',
      durationMinutes: 240,
      reason: 'Migration',
    })
  })

  it('describes a silence with its remaining time', () => {
    expect(formatRemaining(30)).toBe("moins d'une minute")
    expect(formatRemaining(45 * 60)).toBe('45 min')
    expect(formatRemaining(125 * 60)).toBe('2 h 05')
    expect(formatRemaining(76 * 3600)).toBe('3 j 4 h')
    const row = toSilenceRow(silence, now)
    expect(row.scope).toBe('Cible')
    expect(row.subject).toBe(
      'Règle « Disque presque plein » · host:filesystem.percent{mountpoint=/}',
    )
    expect(row.status).toEqual({ label: 'Encore 2 h 05', tone: 'accent' })
    expect(row.liftable).toBe(true)
    const lifted = toSilenceRow(
      {
        ...silence,
        active: false,
        liftedAt: '2026-11-24T09:30:00Z',
        liftedBy: 'admin',
        ruleName: null,
      },
      now,
    )
    expect(lifted.status.label).toBe('Levé par admin')
    expect(lifted.subject).toContain('règle supprimée')
    expect(toSilenceRow({ ...silence, scope: 'all', active: false }, now).subject).toBe(
      'Toutes les alertes',
    )
  })
})

describe('channels', () => {
  it('translates the delivery errors and the status', () => {
    expect(deliveryErrorText('http_status', 401)).toBe('Réponse HTTP 401 du serveur')
    expect(deliveryErrorText('smtp_rejected', 550)).toBe(
      'Refus du serveur de messagerie (code 550)',
    )
    expect(deliveryErrorText('timeout', null)).toBe('Délai dépassé (10 s)')
    expect(channelStatus({ ...channel, lastSentAt: null }).label).toBe('Jamais utilisé')
    expect(channelStatus(channel).label).toBe('Fonctionne')
    expect(
      channelStatus({ ...channel, lastErrorAt: '2026-11-24T09:30:00Z', lastError: 'timeout' })
        .label,
    ).toBe('En erreur')
  })

  it('describes a channel without its secrets', () => {
    const row = toChannelRow({
      ...channel,
      lastErrorAt: '2026-11-24T09:30:00Z',
      lastError: 'http_status',
      lastErrorStatus: 500,
    })
    expect(row.type).toBe('E-mail (SMTP)')
    expect(row.destination).toBe('a@example.org, b@example.org via smtp.example.org:587')
    expect(row.routing).toBe('Avertissement et plus grave · renotifie toutes les 30 min')
    expect(row.secrets).toEqual([
      { name: 'password', label: 'Mot de passe', text: 'défini · 3f9a0c1b7e42' },
    ])
    expect(row.lastError).toContain('Réponse HTTP 500 du serveur')
  })

  it('converts the form to the request: kept, replaced and removed secrets', () => {
    const values = channelToValues(channel)
    expect(values.to).toBe('a@example.org\nb@example.org')
    expect(values.definedSecrets).toEqual(['password'])
    expect(channelInput(values)).toEqual({
      name: 'Mail',
      type: 'smtp',
      enabled: true,
      minSeverity: 'warning',
      renotifyMinutes: 30,
      settings: {
        host: 'smtp.example.org',
        port: 587,
        security: 'starttls',
        username: 'bot',
        from: 'bot@example.org',
        to: ['a@example.org', 'b@example.org'],
      },
      secrets: {},
    })
    expect(channelInput({ ...values, password: 'new' }).secrets).toEqual({ password: 'new' })
    expect(channelInput({ ...values, username: '' }).secrets).toEqual({ password: null })
    const ntfy = { ...emptyChannelValues, name: 'n', serverUrl: 'https://ntfy.sh', topic: 't' }
    expect(
      channelInput({ ...ntfy, definedSecrets: ['accessToken'], removeSecrets: ['accessToken'] })
        .secrets,
    ).toEqual({
      accessToken: null,
    })
    expect(channelInput(ntfy).settings).toEqual({ serverUrl: 'https://ntfy.sh', topic: 't' })
    expect(toggledChannelInput(channel)).toMatchObject({ enabled: false, secrets: {} })
  })
})
