import type { CeremonyOptions } from '../webauthn'
import { requestJSON } from './client'

/** Passkey or security key of the account. */
export type WebAuthnKey = {
  id: string
  name: string
  createdAt: string
  lastUsedAt: string | null
}

/** Second factor of the signed-in account (API_CONTRACTS.md § 3.5). */
export type SecondFactorStatus = {
  enabled: boolean
  pending: boolean
  recoveryCodesRemaining: number
  /** False when the secrets key is missing: TOTP cannot be enrolled. */
  available: boolean
  keys: WebAuthnKey[]
  /** False without webauthn_rp_id: keys cannot be added. */
  webauthnAvailable: boolean
}

export type TotpEnrollment = {
  /** Base32 secret, to type when the QR code cannot be scanned. */
  secret: string
  uri: string
  qr: { size: number; rows: string[] }
}

export function getSecondFactorStatus() {
  return requestJSON<SecondFactorStatus>('/api/v1/auth/second-factor')
}

export function beginTotp(password: string) {
  return requestJSON<TotpEnrollment>('/api/v1/auth/second-factor/totp', {
    method: 'POST',
    body: JSON.stringify({ password }),
  })
}

export function confirmTotp(code: string) {
  return requestJSON<{ recoveryCodes: string[] }>('/api/v1/auth/second-factor/totp/confirm', {
    method: 'POST',
    body: JSON.stringify({ code }),
  }).then((body) => body.recoveryCodes)
}

export function regenerateRecoveryCodes(password: string) {
  return requestJSON<{ recoveryCodes: string[] }>('/api/v1/auth/second-factor/recovery-codes', {
    method: 'POST',
    body: JSON.stringify({ password }),
  }).then((body) => body.recoveryCodes)
}

export function disableSecondFactor(password: string) {
  return requestJSON<{ disabled: boolean }>('/api/v1/auth/second-factor/disable', {
    method: 'POST',
    body: JSON.stringify({ password }),
  })
}

export function beginWebAuthn(password: string) {
  return requestJSON<{ options: CeremonyOptions }>('/api/v1/auth/second-factor/webauthn', {
    method: 'POST',
    body: JSON.stringify({ password }),
  }).then((body) => body.options)
}

/** Stores the key; recoveryCodes is empty unless it is the first second factor. */
export function confirmWebAuthn(name: string, response: unknown) {
  return requestJSON<{ key: WebAuthnKey; recoveryCodes: string[] }>(
    '/api/v1/auth/second-factor/webauthn/confirm',
    { method: 'POST', body: JSON.stringify({ name, response }) },
  )
}

export function deleteWebAuthn(id: string, password: string) {
  return requestJSON<{ deleted: boolean }>(
    `/api/v1/auth/second-factor/webauthn/${encodeURIComponent(id)}/delete`,
    { method: 'POST', body: JSON.stringify({ password }) },
  )
}
