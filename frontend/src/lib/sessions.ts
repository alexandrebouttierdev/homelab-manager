import type { SessionInfo } from './api/security'
import { formatDateTime, formatRelativeTime } from './format'

const browsers: Array<[RegExp, string]> = [
  [/Edg\//, 'Edge'],
  [/Firefox\//, 'Firefox'],
  [/Chrome\//, 'Chrome'],
  [/Safari\//, 'Safari'],
  [/^curl\//i, 'curl'],
]

// Android before Linux and iOS before macOS: their user agents contain both.
const systems: Array<[RegExp, string]> = [
  [/Android/, 'Android'],
  [/iPhone|iPad/, 'iOS'],
  [/Windows/, 'Windows'],
  [/Mac OS X/, 'macOS'],
  [/Linux/, 'Linux'],
]

/** Short description of a browser from its user agent. */
export function browserLabel(userAgent: string): string {
  if (!userAgent) return 'Navigateur inconnu'
  const browser = browsers.find(([pattern]) => pattern.test(userAgent))?.[1] ?? 'Autre client'
  const system = systems.find(([pattern]) => pattern.test(userAgent))?.[1]
  return system ? `${browser} · ${system}` : browser
}

const authMethods: Record<string, string> = { password: 'Mot de passe' }

export type SessionRow = {
  id: string
  username: string
  browser: string
  ipAddress: string
  authMethod: string
  lastSeen: string
  createdAt: string
  current: boolean
}

export function toSessionRow(session: SessionInfo): SessionRow {
  return {
    id: session.id,
    username: session.username,
    browser: browserLabel(session.userAgent),
    ipAddress: session.ipAddress || '—',
    authMethod: authMethods[session.authMethod] ?? session.authMethod,
    lastSeen: formatRelativeTime(session.lastSeenAt),
    createdAt: formatDateTime(session.createdAt),
    current: session.current,
  }
}
