export type SessionUser = {
  id: string
  username: string
}

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message)
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    credentials: 'same-origin',
    headers: {
      'Content-Type': 'application/json',
      ...init?.headers,
    },
  })
  if (!response.ok) {
    const body = await response.json().catch(() => null) as
      { error?: { message?: string } } | null
    throw new ApiError(response.status, body?.error?.message ?? 'Une erreur est survenue.')
  }
  return response.json() as Promise<T>
}

export function login(username: string, password: string) {
  return request<{ user: SessionUser }>('/api/v1/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
}

export function getSession() {
  return request<{ user: SessionUser }>('/api/v1/auth/session')
}

export function logout() {
  return request<{ loggedOut: boolean }>('/api/v1/auth/logout', { method: 'POST' })
}
