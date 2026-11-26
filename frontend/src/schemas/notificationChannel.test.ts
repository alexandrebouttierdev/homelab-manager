import { describe, expect, it } from 'vitest'
import {
  channelSchema,
  isServerURL,
  splitRecipients,
  type ChannelValues,
} from './notificationChannel'

const base: ChannelValues = {
  name: 'Canal',
  type: 'ntfy',
  enabled: true,
  minSeverity: 'warning',
  renotifyMinutes: '30',
  chatId: '',
  serverUrl: 'https://ntfy.sh',
  topic: 'monoops',
  host: '',
  port: '587',
  security: 'starttls',
  username: '',
  from: '',
  to: '',
  url: '',
  botToken: '',
  webhookUrl: '',
  appToken: '',
  accessToken: '',
  password: '',
  signingSecret: '',
  definedSecrets: [],
  removeSecrets: [],
}

function errors(values: Partial<ChannelValues>): string[] {
  const result = channelSchema.safeParse({ ...base, ...values })
  return result.success ? [] : result.error.issues.map((issue) => String(issue.path[0]))
}

describe('channelSchema', () => {
  it('accepts a valid channel of each type', () => {
    expect(errors({})).toEqual([])
    expect(
      errors({ type: 'telegram', chatId: '-100123', botToken: `123:${'A'.repeat(35)}` }),
    ).toEqual([])
    expect(errors({ type: 'discord', webhookUrl: 'https://discord.com/api/webhooks/1/x' })).toEqual(
      [],
    )
    expect(
      errors({ type: 'gotify', serverUrl: 'http://192.168.1.10:8080', appToken: 't' }),
    ).toEqual([])
    expect(
      errors({
        type: 'smtp',
        host: 'smtp.example.org',
        from: 'bot@example.org',
        to: 'a@example.org, b@example.org',
        username: 'bot',
        password: 'secret',
      }),
    ).toEqual([])
    expect(errors({ type: 'webhook', url: 'https://hooks.example.org/x' })).toEqual([])
  })

  it('requires the secrets unless they are already defined and kept', () => {
    expect(errors({ type: 'gotify', serverUrl: 'https://g.example' })).toEqual(['appToken'])
    expect(
      errors({ type: 'gotify', serverUrl: 'https://g.example', definedSecrets: ['appToken'] }),
    ).toEqual([])
    expect(
      errors({
        type: 'gotify',
        serverUrl: 'https://g.example',
        definedSecrets: ['appToken'],
        removeSecrets: ['appToken'],
      }),
    ).toEqual(['appToken'])
  })

  it('refuses invalid settings', () => {
    expect(errors({ renotifyMinutes: '3' })).toEqual(['renotifyMinutes'])
    expect(errors({ renotifyMinutes: '0' })).toEqual([])
    expect(errors({ topic: 'a/b' })).toEqual(['topic'])
    expect(errors({ serverUrl: 'https://u:p@ntfy.sh' })).toEqual(['serverUrl'])
    expect(errors({ type: 'telegram', chatId: 'chat', botToken: 'bad' })).toEqual([
      'chatId',
      'botToken',
    ])
    expect(errors({ type: 'discord', webhookUrl: 'https://evil.example/api/webhooks/1' })).toEqual([
      'webhookUrl',
    ])
    expect(
      errors({
        type: 'smtp',
        host: 'smtp example',
        port: '70000',
        from: 'Bot <b@x.org>',
        to: 'a@x.org a@x.org',
        username: 'bot',
      }),
    ).toEqual(['host', 'port', 'from', 'to', 'password'])
    expect(errors({ type: 'webhook', url: 'file:///etc', signingSecret: 'short' })).toEqual([
      'url',
      'signingSecret',
    ])
  })

  it('checks URLs and splits recipients', () => {
    expect(isServerURL('https://ntfy.sh/base')).toBe(true)
    expect(isServerURL('https://ntfy.sh/?topic=x')).toBe(false)
    expect(isServerURL('ftp://x.org')).toBe(false)
    expect(splitRecipients(' a@x.org,\nb@x.org ; ')).toEqual(['a@x.org', 'b@x.org'])
  })
})
