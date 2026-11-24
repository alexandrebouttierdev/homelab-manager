import { z } from 'zod'
import { hasControlCharacter } from './silence'

// Bounds of API_CONTRACTS.md § 4.8 (channels); the server remains the
// reference. A secret left empty keeps the stored value (definedSecrets)
// unless it is listed in removeSecrets.
const telegramChat = /^(-?\d{1,20}|@[A-Za-z0-9_]{5,32})$/
const telegramToken = /^\d{1,20}:[A-Za-z0-9_-]{30,100}$/
const ntfyTopic = /^[A-Za-z0-9_-]{1,64}$/
const hostname =
  /^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$|^[0-9a-fA-F:.]+$/
const email = /^[^\s@<>,;"]+@[^\s@<>,;"]+$/
const discordWebhook = /^https:\/\/(discord\.com|discordapp\.com)\/api\/webhooks\//

/** An http or https URL without credentials, query nor fragment. */
export function isServerURL(value: string): boolean {
  if (value.length > 2048 || /\s/.test(value)) return false
  try {
    const url = new URL(value)
    return (
      (url.protocol === 'http:' || url.protocol === 'https:') &&
      url.hostname !== '' &&
      url.username === '' &&
      url.password === '' &&
      url.search === '' &&
      url.hash === '' &&
      !value.includes('?') &&
      !value.includes('#')
    )
  } catch {
    return false
  }
}

/** Splits the recipients typed one per line or separated by commas. */
export function splitRecipients(value: string): string[] {
  return value
    .split(/[\s,;]+/)
    .map((address) => address.trim())
    .filter((address) => address !== '')
}

export const channelSchema = z
  .object({
    name: z
      .string()
      .trim()
      .min(1, 'Nom requis')
      .max(100, 'Maximum 100 caractères')
      .refine((value) => !hasControlCharacter(value), 'Une seule ligne'),
    type: z.enum(['telegram', 'discord', 'gotify', 'ntfy', 'smtp', 'webhook']),
    enabled: z.boolean(),
    minSeverity: z.enum(['info', 'warning', 'critical']),
    renotifyMinutes: z.string(),
    chatId: z.string(),
    serverUrl: z.string(),
    topic: z.string(),
    host: z.string(),
    port: z.string(),
    security: z.enum(['starttls', 'tls']),
    username: z.string().max(255, 'Maximum 255 caractères'),
    from: z.string(),
    to: z.string(),
    url: z.string(),
    botToken: z.string(),
    webhookUrl: z.string(),
    appToken: z.string(),
    accessToken: z.string(),
    password: z.string(),
    signingSecret: z.string(),
    definedSecrets: z.array(z.string()),
    removeSecrets: z.array(z.string()),
  })
  .superRefine((values, ctx) => {
    const issue = (path: string, message: string) =>
      ctx.addIssue({ code: 'custom', path: [path], message })
    const secret = (name: 'botToken' | 'webhookUrl' | 'appToken' | 'password') =>
      values[name].trim() !== '' ||
      (values.definedSecrets.includes(name) && !values.removeSecrets.includes(name))
    for (const name of [
      'botToken',
      'webhookUrl',
      'appToken',
      'accessToken',
      'password',
      'signingSecret',
    ] as const) {
      const value = values[name]
      if (value.length > 1024 || hasControlCharacter(value))
        issue(name, '1 024 caractères au plus, sur une ligne')
    }
    const renotify = Number(values.renotifyMinutes)
    if (
      !/^\d+$/.test(values.renotifyMinutes.trim()) ||
      (renotify !== 0 && (renotify < 5 || renotify > 1440))
    )
      issue('renotifyMinutes', '0 (jamais) ou de 5 à 1 440 minutes')
    switch (values.type) {
      case 'telegram':
        if (!telegramChat.test(values.chatId.trim()))
          issue('chatId', 'Identifiant numérique du chat ou @canal')
        if (!secret('botToken')) issue('botToken', 'Jeton du bot requis')
        else if (values.botToken.trim() && !telegramToken.test(values.botToken.trim()))
          issue('botToken', 'Format attendu : 123456789:ABC…')
        break
      case 'discord':
        if (!secret('webhookUrl')) issue('webhookUrl', 'URL du webhook requise')
        else if (values.webhookUrl.trim() && !discordWebhook.test(values.webhookUrl.trim()))
          issue('webhookUrl', 'URL https://discord.com/api/webhooks/…')
        break
      case 'gotify':
        if (!isServerURL(values.serverUrl.trim())) issue('serverUrl', 'URL http ou https')
        if (!secret('appToken')) issue('appToken', "Jeton d'application requis")
        break
      case 'ntfy':
        if (!isServerURL(values.serverUrl.trim())) issue('serverUrl', 'URL http ou https')
        if (!ntfyTopic.test(values.topic.trim()))
          issue('topic', 'Lettres, chiffres, - et _ (64 au plus)')
        break
      case 'smtp': {
        if (!hostname.test(values.host.trim()) || values.host.length > 255)
          issue('host', "Nom d'hôte ou adresse IP")
        const port = Number(values.port)
        if (!/^\d+$/.test(values.port.trim()) || port < 1 || port > 65535)
          issue('port', 'Port de 1 à 65 535')
        if (!email.test(values.from.trim())) issue('from', 'Adresse électronique')
        const recipients = splitRecipients(values.to)
        if (
          recipients.length === 0 ||
          recipients.length > 10 ||
          new Set(recipients).size !== recipients.length ||
          !recipients.every((address) => email.test(address))
        )
          issue('to', '1 à 10 adresses distinctes')
        if (values.username.trim() !== '' && !secret('password'))
          issue('password', "Mot de passe requis avec un nom d'utilisateur")
        if (values.username.trim() === '' && values.password.trim() !== '')
          issue('username', "Nom d'utilisateur requis avec un mot de passe")
        break
      }
      case 'webhook':
        if (!isServerURL(values.url.trim())) issue('url', 'URL http ou https')
        if (values.signingSecret.trim() !== '' && values.signingSecret.trim().length < 16)
          issue('signingSecret', '16 caractères au moins')
        break
    }
  })

export type ChannelValues = z.infer<typeof channelSchema>
