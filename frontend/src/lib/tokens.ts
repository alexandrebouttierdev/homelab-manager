import type { BadgeTone } from '../components/ui/Badge'
import type { Role } from './api/auth'
import type { ApiToken, TokenScope } from './api/tokens'
import { canManageContainers, canManageServices } from './roles'

const scopeLabels: Record<TokenScope, string> = {
  'metrics:read': 'Métriques et synthèse',
  'containers:read': 'Conteneurs',
  'events:read': 'Événements',
  'services:read': 'Services système',
  'backups:read': 'Sauvegardes',
  'services:operate': 'Agir sur les services',
  'containers:operate': 'Agir sur les conteneurs',
}

/** Action scopes (D-4) and the role check that offers them; the server checks again. */
const actionScopes: Partial<Record<TokenScope, (role: Role | undefined) => boolean>> = {
  'services:operate': canManageServices,
  'containers:operate': canManageContainers,
}

export function isActionScope(scope: TokenScope): boolean {
  return scope in actionScopes
}

/** Scopes offered to the role: read scopes always, action scopes if the role may act. */
export function tokenScopeOptions(
  role: Role | undefined,
): Array<{ value: TokenScope; label: string; action: boolean }> {
  return (Object.keys(scopeLabels) as TokenScope[])
    .filter((value) => actionScopes[value]?.(role) ?? true)
    .map((value) => ({ value, label: scopeLabels[value], action: isActionScope(value) }))
}

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
