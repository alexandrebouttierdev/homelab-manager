import type { Role } from './auth'
import { requestJSON } from './client'

/** A Manager account as returned to administrators (API_CONTRACTS.md § 3.1). */
export type Account = {
  id: string
  username: string
  displayName: string
  role: Role
  disabled: boolean
  mustChangePassword: boolean
  lastLoginAt: string | null
  lastLoginIp: string | null
  createdAt: string
}

export type NewAccount = {
  username: string
  displayName: string
  role: Role
  password: string
}

export type AccountChange = {
  displayName?: string
  role?: Role
  disabled?: boolean
}

export function listAccounts() {
  return requestJSON<{ users: Account[] }>('/api/v1/users').then((body) => body.users)
}

export function createAccount(input: NewAccount) {
  return requestJSON<{ user: Account }>('/api/v1/users', {
    method: 'POST',
    body: JSON.stringify(input),
  }).then((body) => body.user)
}

export function updateAccount(id: string, change: AccountChange) {
  return requestJSON<{ user: Account }>(`/api/v1/users/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: JSON.stringify(change),
  }).then((body) => body.user)
}

export function deleteAccount(id: string) {
  return requestJSON<{ deleted: boolean }>(`/api/v1/users/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}
