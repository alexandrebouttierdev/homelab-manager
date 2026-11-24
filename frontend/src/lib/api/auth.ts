import type { CeremonyOptions } from '../webauthn'
import { requestJSON } from './client'

export { ApiError } from './client'

export type Role = 'viewer' | 'operator' | 'admin'

export type SessionUser = {
  id: string
  username: string
  displayName: string
  role: Role
  /** The account must choose its own password before anything else. */
  mustChangePassword: boolean
  /** The policy requires a second factor and this administrator has none. */
  mustEnrollSecondFactor: boolean
}

export type SecondFactorMethod = 'totp' | 'webauthn' | 'recovery'

/** A password accepted for an account that still has to give its second factor. */
export type SecondFactorChallenge = {
  secondFactorRequired: true
  methods: SecondFactorMethod[]
  expiresAt: string
}

export type LoginResponse = { user: SessionUser } | SecondFactorChallenge

export function needsSecondFactor(response: LoginResponse): response is SecondFactorChallenge {
  return 'secondFactorRequired' in response
}

export function login(username: string, password: string) {
  return requestJSON<LoginResponse>('/api/v1/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
}

export function getSession() {
  return requestJSON<{ user: SessionUser }>('/api/v1/auth/session')
}

export function logout() {
  return requestJSON<{ loggedOut: boolean }>('/api/v1/auth/logout', { method: 'POST' })
}

export function changePassword(currentPassword: string, newPassword: string) {
  return requestJSON<{ changed: boolean }>('/api/v1/auth/password', {
    method: 'POST',
    body: JSON.stringify({ currentPassword, newPassword }),
  })
}

/** Second step of a login, with the challenge cookie set by login. */
export function completeSecondFactor(method: SecondFactorMethod, code: string) {
  return requestJSON<{ user: SessionUser }>('/api/v1/auth/login/second-factor', {
    method: 'POST',
    body: JSON.stringify({ method, code }),
  })
}

/** Options of a key login, for the pending login of the challenge cookie. */
export function beginWebAuthnLogin() {
  return requestJSON<{ options: CeremonyOptions }>('/api/v1/auth/login/second-factor/webauthn', {
    method: 'POST',
  }).then((body) => body.options)
}

/** Second step of a login with the answer of a security key. */
export function completeWithKey(response: unknown) {
  return requestJSON<{ user: SessionUser }>('/api/v1/auth/login/second-factor', {
    method: 'POST',
    body: JSON.stringify({ method: 'webauthn', response }),
  })
}
