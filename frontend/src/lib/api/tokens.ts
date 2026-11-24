import { requestJSON } from './client'

/** Read-only scopes of an API token (API_CONTRACTS.md § 3.6). */
export type TokenScope =
  'metrics:read' | 'containers:read' | 'events:read' | 'services:read' | 'backups:read'

/** A token without its secret, which is shown once at creation. */
export type ApiToken = {
  id: string
  name: string
  prefix: string
  scopes: TokenScope[]
  username: string
  createdAt: string
  expiresAt: string
  lastUsedAt: string | null
}

export type ApiTokenInput = { name: string; scopes: TokenScope[]; expiresInDays: number }

export async function listApiTokens() {
  return requestJSON<{ tokens: ApiToken[]; scopes: TokenScope[] }>('/api/v1/auth/tokens')
}

export function createApiToken(input: ApiTokenInput) {
  return requestJSON<{ token: string; apiToken: ApiToken }>('/api/v1/auth/tokens', {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

export function revokeApiToken(id: string) {
  return requestJSON<void>(`/api/v1/auth/tokens/${encodeURIComponent(id)}`, { method: 'DELETE' })
}
