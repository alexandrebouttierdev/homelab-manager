import type { BadgeTone } from '../components/ui/Badge'
import type { ApiToken, TokenScope } from './api/tokens'

const scopeLabels: Record<TokenScope, string> = {
  'metrics:read': 'Métriques et synthèse',
  'containers:read': 'Conteneurs',
  'events:read': 'Événements',
  'services:read': 'Services système',
  'backups:read': 'Sauvegardes',
}

export const tokenScopeOptions: ReadonlyArray<{ value: TokenScope; label: string }> = (
  Object.keys(scopeLabels) as TokenScope[]
).map((value) => ({ value, label: scopeLabels[value] }))

export function tokenScopeLabel(scope: TokenScope): string {
  return scopeLabels[scope]
}

const dayMs = 24 * 60 * 60 * 1000

/** Expiry state of a token: expired, close (7 days or less) or valid. */
export function tokenExpiry(token: ApiToken, now: Date): { label: string; tone: BadgeTone } {
  const left = new Date(token.expiresAt).getTime() - now.getTime()
  if (left <= 0) return { label: 'Expiré', tone: 'danger' }
  const days = Math.ceil(left / dayMs)
  if (days <= 7) return { label: `Expire dans ${days} j`, tone: 'warning' }
  return { label: `Expire dans ${days} j`, tone: 'neutral' }
}
