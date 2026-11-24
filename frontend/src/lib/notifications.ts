import type { BadgeTone } from '../components/ui/Badge'
import type { Alert } from './api/alerts'
import type {
  ChannelInput,
  ChannelSettings,
  ChannelType,
  DeliveryError,
  NotificationChannel,
  SecretName,
  Silence,
  SilenceInput,
  SilenceScope,
} from './api/notifications'
import { describeTarget, severityLabel, severityOptions } from './alerts'
import { formatDateTime } from './format'
import { splitRecipients, type ChannelValues } from '../schemas/notificationChannel'
import type { SilenceValues } from '../schemas/silence'

// --- Silences (API_CONTRACTS.md § 4.8) ---

export const silenceDurationOptions: ReadonlyArray<{
  value: SilenceValues['durationMinutes']
  label: string
}> = [
  { value: '15', label: '15 minutes' },
  { value: '60', label: '1 heure' },
  { value: '240', label: '4 heures' },
  { value: '1440', label: '1 jour' },
  { value: '4320', label: '3 jours' },
  { value: '10080', label: '7 jours' },
]

/** What a silence is posed on: one alert (its rule or its target), or every alert. */
export type SilenceSubject = {
  ruleId: string
  ruleName: string
  targetKey: string
  target: string
} | null

export function silenceSubject(alert: Alert): NonNullable<SilenceSubject> {
  return {
    ruleId: alert.ruleId,
    ruleName: alert.ruleName,
    targetKey: alert.targetKey,
    target: describeTarget(alert),
  }
}

export function silenceScopeOptions(
  subject: SilenceSubject,
): ReadonlyArray<{ value: SilenceScope; label: string }> {
  if (!subject) return [{ value: 'all', label: 'Toutes les alertes' }]
  return [
    { value: 'target', label: `Cette alerte seulement (${subject.target})` },
    { value: 'rule', label: `Toutes les alertes de la règle « ${subject.ruleName} »` },
  ]
}

export const emptySilenceValues = (subject: SilenceSubject): SilenceValues => ({
  scope: subject ? 'target' : 'all',
  durationMinutes: '60',
  reason: '',
})

export function silenceInput(values: SilenceValues, subject: SilenceSubject): SilenceInput {
  const common = { durationMinutes: Number(values.durationMinutes), reason: values.reason.trim() }
  if (!subject || values.scope === 'all') return { scope: 'all', ...common }
  if (values.scope === 'rule') return { scope: 'rule', ruleId: subject.ruleId, ...common }
  return { scope: 'target', ruleId: subject.ruleId, targetKey: subject.targetKey, ...common }
}

/** Remaining time of a silence, rounded down: "45 min", "2 h 05", "3 j 4 h". */
export function formatRemaining(seconds: number): string {
  const minutes = Math.floor(seconds / 60)
  if (minutes < 1) return "moins d'une minute"
  if (minutes < 60) return `${minutes} min`
  if (minutes < 1440)
    return `${Math.floor(minutes / 60)} h ${String(minutes % 60).padStart(2, '0')}`
  return `${Math.floor(minutes / 1440)} j ${Math.floor((minutes % 1440) / 60)} h`
}

export type SilenceRow = {
  id: string
  scope: string
  subject: string
  reason: string
  createdBy: string
  period: string
  status: { label: string; tone: BadgeTone }
  liftable: boolean
}

export function toSilenceRow(silence: Silence, now: Date): SilenceRow {
  const rule = silence.ruleName ? `« ${silence.ruleName} »` : 'règle supprimée'
  const subject = {
    all: 'Toutes les alertes',
    rule: `Règle ${rule}`,
    target: `Règle ${rule} · ${silence.targetKey ?? ''}`,
  }[silence.scope]
  const ended = silence.liftedAt ?? silence.endsAt
  let status: SilenceRow['status']
  if (silence.active) {
    const remaining = (new Date(silence.endsAt).getTime() - now.getTime()) / 1000
    status = { label: `Encore ${formatRemaining(Math.max(remaining, 0))}`, tone: 'accent' }
  } else if (silence.liftedAt) {
    status = { label: `Levé par ${silence.liftedBy ?? '—'}`, tone: 'neutral' }
  } else {
    status = { label: 'Terminé', tone: 'neutral' }
  }
  return {
    id: silence.id,
    scope: { all: 'Global', rule: 'Règle', target: 'Cible' }[silence.scope],
    subject,
    reason: silence.reason,
    createdBy: silence.createdBy,
    period: `${formatDateTime(silence.startsAt)} → ${formatDateTime(ended)}`,
    status,
    liftable: silence.active,
  }
}

// --- Channels (API_CONTRACTS.md § 4.8) ---

const channelTypeLabels: Record<ChannelType, string> = {
  telegram: 'Telegram',
  discord: 'Discord',
  gotify: 'Gotify',
  ntfy: 'ntfy',
  smtp: 'E-mail (SMTP)',
  webhook: 'Webhook',
}

export const channelTypeOptions: ReadonlyArray<{ value: ChannelType; label: string }> = (
  Object.keys(channelTypeLabels) as ChannelType[]
).map((value) => ({ value, label: channelTypeLabels[value] }))

export const minSeverityOptions = severityOptions.map((option) => ({
  value: option.value,
  label: `${option.label} et plus grave`,
}))

export type SecretField = {
  name: SecretName
  label: string
  required: boolean
  hint: string
}

const secretFields: Record<ChannelType, SecretField[]> = {
  telegram: [
    { name: 'botToken', label: 'Jeton du bot', required: true, hint: 'Donné par @BotFather' },
  ],
  discord: [
    {
      name: 'webhookUrl',
      label: 'URL du webhook',
      required: true,
      hint: 'https://discord.com/api/webhooks/…',
    },
  ],
  gotify: [
    {
      name: 'appToken',
      label: "Jeton d'application",
      required: true,
      hint: 'Créé dans Gotify',
    },
  ],
  ntfy: [
    {
      name: 'accessToken',
      label: "Jeton d'accès",
      required: false,
      hint: 'Seulement pour un sujet protégé',
    },
  ],
  smtp: [
    {
      name: 'password',
      label: 'Mot de passe',
      required: false,
      hint: "Avec un nom d'utilisateur seulement",
    },
  ],
  webhook: [
    {
      name: 'signingSecret',
      label: 'Secret de signature',
      required: false,
      hint: 'HMAC-SHA256, 16 caractères au moins',
    },
  ],
}

export function channelSecretFields(type: ChannelType): SecretField[] {
  return secretFields[type]
}

/** French text of a delivery failure; the API keeps no address nor answer. */
export function deliveryErrorText(code: DeliveryError, status: number | null): string {
  switch (code) {
    case 'timeout':
      return 'Délai dépassé (10 s)'
    case 'connection_failed':
      return 'Connexion impossible'
    case 'tls_failed':
      return 'Échec TLS : certificat non reconnu ou chiffrement indisponible'
    case 'redirect_refused':
      return `Redirection refusée${status ? ` (HTTP ${status})` : ''}`
    case 'http_status':
      return `Réponse HTTP ${status ?? '—'} du serveur`
    case 'smtp_rejected':
      return `Refus du serveur de messagerie${status ? ` (code ${status})` : ''}`
    case 'secrets_unavailable':
      return 'Clé du coffre absente : secrets illisibles'
    default:
      return "Échec de l'envoi"
  }
}

export function channelStatus(channel: NotificationChannel): { label: string; tone: BadgeTone } {
  const sent = channel.lastSentAt ? new Date(channel.lastSentAt).getTime() : null
  const failed = channel.lastErrorAt ? new Date(channel.lastErrorAt).getTime() : null
  if (sent === null && failed === null) return { label: 'Jamais utilisé', tone: 'neutral' }
  if (failed !== null && (sent === null || failed > sent))
    return { label: 'En erreur', tone: 'danger' }
  return { label: 'Fonctionne', tone: 'success' }
}

function destination(channel: NotificationChannel): string {
  const s = channel.settings
  switch (channel.type) {
    case 'telegram':
      return `Chat ${s.chatId ?? '—'}`
    case 'discord':
      return 'Webhook Discord'
    case 'gotify':
      return s.serverUrl ?? '—'
    case 'ntfy':
      return `${s.serverUrl ?? '—'} · ${s.topic ?? '—'}`
    case 'smtp':
      return `${(s.to ?? []).join(', ')} via ${s.host ?? '—'}:${s.port ?? '—'}`
    case 'webhook':
      return s.url ?? '—'
  }
}

export type ChannelRow = {
  id: string
  name: string
  type: string
  destination: string
  routing: string
  enabled: boolean
  status: { label: string; tone: BadgeTone }
  lastSent: string
  lastError: string | null
  secrets: Array<{ name: SecretName; label: string; text: string }>
}

export function toChannelRow(channel: NotificationChannel): ChannelRow {
  const renotify =
    channel.renotifyMinutes === 0
      ? 'sans renotification'
      : `renotifie toutes les ${channel.renotifyMinutes} min`
  return {
    id: channel.id,
    name: channel.name,
    type: channelTypeLabels[channel.type] ?? channel.type,
    destination: destination(channel),
    routing: `${severityLabel(channel.minSeverity)} et plus grave · ${renotify}`,
    enabled: channel.enabled,
    status: channelStatus(channel),
    lastSent: channel.lastSentAt ? formatDateTime(channel.lastSentAt) : '—',
    lastError:
      channel.lastError && channel.lastErrorAt
        ? `${formatDateTime(channel.lastErrorAt)} · ${deliveryErrorText(channel.lastError, channel.lastErrorStatus)}`
        : null,
    secrets: secretFields[channel.type].map((field) => {
      const secret = channel.secrets[field.name]
      let text = 'non défini'
      if (secret?.defined) text = secret.fingerprint ? `défini · ${secret.fingerprint}` : 'défini'
      return { name: field.name, label: field.label, text }
    }),
  }
}

const noSecrets = {
  botToken: '',
  webhookUrl: '',
  appToken: '',
  accessToken: '',
  password: '',
  signingSecret: '',
}

export const emptyChannelValues: ChannelValues = {
  name: '',
  type: 'ntfy',
  enabled: true,
  minSeverity: 'warning',
  renotifyMinutes: '30',
  chatId: '',
  serverUrl: '',
  topic: '',
  host: '',
  port: '587',
  security: 'starttls',
  username: '',
  from: '',
  to: '',
  url: '',
  ...noSecrets,
  definedSecrets: [],
  removeSecrets: [],
}

/** Form values of a stored channel; secrets stay empty (kept unless typed). */
export function channelToValues(channel: NotificationChannel): ChannelValues {
  const s = channel.settings
  return {
    ...emptyChannelValues,
    name: channel.name,
    type: channel.type,
    enabled: channel.enabled,
    minSeverity: channel.minSeverity,
    renotifyMinutes: String(channel.renotifyMinutes),
    chatId: s.chatId ?? '',
    serverUrl: s.serverUrl ?? '',
    topic: s.topic ?? '',
    host: s.host ?? '',
    port: s.port ? String(s.port) : '587',
    security: s.security ?? 'starttls',
    username: s.username ?? '',
    from: s.from ?? '',
    to: (s.to ?? []).join('\n'),
    url: s.url ?? '',
    definedSecrets: (Object.keys(channel.secrets) as SecretName[]).filter(
      (name) => channel.secrets[name]?.defined,
    ),
  }
}

function settingsOf(values: ChannelValues): ChannelSettings {
  switch (values.type) {
    case 'telegram':
      return { chatId: values.chatId.trim() }
    case 'discord':
      return {}
    case 'gotify':
      return { serverUrl: values.serverUrl.trim() }
    case 'ntfy':
      return { serverUrl: values.serverUrl.trim(), topic: values.topic.trim() }
    case 'smtp':
      return {
        host: values.host.trim(),
        port: Number(values.port),
        security: values.security,
        ...(values.username.trim() ? { username: values.username.trim() } : {}),
        from: values.from.trim(),
        to: splitRecipients(values.to),
      }
    case 'webhook':
      return { url: values.url.trim() }
  }
}

/** The request body: settings of the type, typed secrets, removed ones as null. */
export function channelInput(values: ChannelValues): ChannelInput {
  const secrets: ChannelInput['secrets'] = {}
  for (const field of secretFields[values.type]) {
    const typed = values[field.name].trim()
    if (typed) secrets[field.name] = typed
    else if (
      values.removeSecrets.includes(field.name) &&
      values.definedSecrets.includes(field.name)
    )
      secrets[field.name] = null
  }
  // Without a username, a stored SMTP password must go as well.
  if (
    values.type === 'smtp' &&
    !values.username.trim() &&
    values.definedSecrets.includes('password')
  )
    secrets.password = null
  return {
    name: values.name.trim(),
    type: values.type,
    enabled: values.enabled,
    minSeverity: values.minSeverity,
    renotifyMinutes: Number(values.renotifyMinutes),
    settings: settingsOf(values),
    secrets,
  }
}

/** The body that switches a channel on or off, secrets unchanged. */
export function toggledChannelInput(channel: NotificationChannel): ChannelInput {
  return {
    name: channel.name,
    type: channel.type,
    enabled: !channel.enabled,
    minSeverity: channel.minSeverity,
    renotifyMinutes: channel.renotifyMinutes,
    settings: channel.settings,
    secrets: {},
  }
}
