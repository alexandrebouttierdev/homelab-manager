import { requestJSON } from './client'

/** An active session (API_CONTRACTS.md § 3.2). */
export type SessionInfo = {
  id: string
  userId: string
  username: string
  ipAddress: string
  userAgent: string
  authMethod: string
  createdAt: string
  lastSeenAt: string
  expiresAt: string
  current: boolean
}

/** Security policy (API_CONTRACTS.md § 3.3). */
export type SecurityPolicy = {
  idleTimeoutMinutes: number
  allowedNetworks: string[]
  /** Administrators without a second factor are limited to its enrollment. */
  requireAdminSecondFactor: boolean
  /** Accounts may sign in with a passkey alone, without their password (decision D-14). */
  allowPasskeyLogin: boolean
  /** Address of this browser as seen by the Manager. */
  currentAddress: string
}

export type SecurityPolicyChange = Omit<SecurityPolicy, 'currentAddress'>

/** Audit log entry (API_CONTRACTS.md § 3.4). */
export type AuditEntry = {
  id: string
  actor: string
  targetType: string
  targetId: string
  action: string
  outcome: string
  requestId: string | null
  createdAt: string
}

export type AuditFilter = {
  actor?: string
  targetType?: string
  action?: string
  outcome?: string
  from?: Date
  to?: Date
  limit?: number
}

function auditQuery(filter: AuditFilter): URLSearchParams {
  const query = new URLSearchParams()
  for (const key of ['actor', 'targetType', 'action', 'outcome'] as const) {
    const value = filter[key]
    if (value) query.set(key, value)
  }
  if (filter.from) query.set('from', filter.from.toISOString())
  if (filter.to) query.set('to', filter.to.toISOString())
  if (filter.limit) query.set('limit', String(filter.limit))
  return query
}

export function listSessions(all: boolean) {
  const path = all ? '/api/v1/auth/sessions?scope=all' : '/api/v1/auth/sessions'
  return requestJSON<{ sessions: SessionInfo[] }>(path).then((body) => body.sessions)
}

export function revokeSession(id: string) {
  return requestJSON<{ revoked: boolean }>(`/api/v1/auth/sessions/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export function revokeOtherSessions() {
  return requestJSON<{ revoked: number }>('/api/v1/auth/sessions/revoke-others', {
    method: 'POST',
  }).then((body) => body.revoked)
}

export function getSecurityPolicy() {
  return requestJSON<SecurityPolicy>('/api/v1/security/policy')
}

export function updateSecurityPolicy(change: SecurityPolicyChange) {
  return requestJSON<SecurityPolicy>('/api/v1/security/policy', {
    method: 'PUT',
    body: JSON.stringify(change),
  })
}

export function listAudit(filter: AuditFilter) {
  const query = auditQuery(filter).toString()
  return requestJSON<{ entries: AuditEntry[] }>(`/api/v1/audit${query ? `?${query}` : ''}`).then(
    (body) => body.entries,
  )
}

/** Same-origin link that downloads the filtered audit log. */
export function auditExportUrl(filter: AuditFilter, format: 'csv' | 'json') {
  const query = auditQuery(filter)
  query.set('format', format)
  return `/api/v1/audit/export?${query.toString()}`
}
