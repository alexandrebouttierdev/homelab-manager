import { requestJSON } from './client'

export { ApiError } from './client'

export type SessionUser = {
  id: string
  username: string
}

export function login(username: string, password: string) {
  return requestJSON<{ user: SessionUser }>('/api/v1/auth/login', {
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
