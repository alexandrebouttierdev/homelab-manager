import type { AlertSeverity } from './alerts'
import { requestJSON } from './client'

/** Alert silences and notification channels (API_CONTRACTS.md § 4.8). */
export type SilenceScope = 'rule' | 'target' | 'all'

export type Silence = {
  id: string
  scope: SilenceScope
  ruleId: string | null
  ruleName: string | null
  targetKey: string | null
  reason: string
  createdBy: string
  startsAt: string
  endsAt: string
  liftedAt: string | null
  liftedBy: string | null
  active: boolean
}

export type SilenceInput = {
  scope: SilenceScope
  ruleId?: string
  targetKey?: string
  durationMinutes: number
  reason: string
}

export type ChannelType = 'telegram' | 'discord' | 'gotify' | 'ntfy' | 'smtp' | 'webhook'
export type SecretName =
  'botToken' | 'webhookUrl' | 'appToken' | 'accessToken' | 'password' | 'signingSecret'
export type DeliveryError =
  | 'timeout'
  | 'connection_failed'
  | 'tls_failed'
  | 'redirect_refused'
  | 'http_status'
  | 'smtp_rejected'
  | 'secrets_unavailable'
  | 'failed'

/** Settings that are not secret; each type uses its own. */
export type ChannelSettings = {
  chatId?: string
  serverUrl?: string
  topic?: string
  host?: string
  port?: number
  security?: 'starttls' | 'tls'
  username?: string
  from?: string
  to?: string[]
  url?: string
}

export type NotificationChannel = {
  id: string
  name: string
  type: ChannelType
  enabled: boolean
  minSeverity: AlertSeverity
  renotifyMinutes: number
  settings: ChannelSettings
  /** Never the values: whether each secret is defined, and its fingerprint. */
  secrets: Partial<Record<SecretName, { defined: boolean; fingerprint: string | null }>>
  lastSentAt: string | null
  lastErrorAt: string | null
  lastError: DeliveryError | null
  lastErrorStatus: number | null
  createdAt: string
  updatedAt: string
}

export type ChannelInput = {
  name: string
  type: ChannelType
  enabled: boolean
  minSeverity: AlertSeverity
  renotifyMinutes: number
  settings: ChannelSettings
  /** Absent: kept; null: removed; a value: replaced. */
  secrets: Partial<Record<SecretName, string | null>>
}

export async function listSilences(): Promise<Silence[]> {
  const body = await requestJSON<{ silences: Silence[] }>('/api/v1/alert-silences')
  return body.silences
}

export async function createSilence(input: SilenceInput): Promise<Silence> {
  const body = await requestJSON<{ silence: Silence }>('/api/v1/alert-silences', {
    method: 'POST',
    body: JSON.stringify(input),
  })
  return body.silence
}

export async function liftSilence(id: string): Promise<Silence> {
  const body = await requestJSON<{ silence: Silence }>(
    `/api/v1/alert-silences/${encodeURIComponent(id)}`,
    { method: 'DELETE' },
  )
  return body.silence
}

export function listChannels(): Promise<{
  channels: NotificationChannel[]
  secretsAvailable: boolean
}> {
  return requestJSON('/api/v1/notification-channels')
}

export async function createChannel(input: ChannelInput): Promise<NotificationChannel> {
  const body = await requestJSON<{ channel: NotificationChannel }>(
    '/api/v1/notification-channels',
    { method: 'POST', body: JSON.stringify(input) },
  )
  return body.channel
}

export async function updateChannel(id: string, input: ChannelInput): Promise<NotificationChannel> {
  const body = await requestJSON<{ channel: NotificationChannel }>(
    `/api/v1/notification-channels/${encodeURIComponent(id)}`,
    { method: 'PUT', body: JSON.stringify(input) },
  )
  return body.channel
}

export async function deleteChannel(id: string): Promise<void> {
  await requestJSON<{ deleted: boolean }>(
    `/api/v1/notification-channels/${encodeURIComponent(id)}`,
    { method: 'DELETE' },
  )
}

export async function testChannel(id: string): Promise<NotificationChannel> {
  const body = await requestJSON<{ channel: NotificationChannel }>(
    `/api/v1/notification-channels/${encodeURIComponent(id)}/test`,
    { method: 'POST' },
  )
  return body.channel
}
