export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public code?: string,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

type ErrorBody = {
  error?: {
    code?: string
    message?: string
  }
}

export async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  let response: Response
  try {
    response = await fetch(path, {
      ...init,
      credentials: 'same-origin',
      headers: {
        Accept: 'application/json',
        ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
        ...init?.headers,
      },
    })
  } catch {
    throw new ApiError(0, 'Impossible de joindre le Manager local.')
  }

  if (!response.ok) {
    const body = await response.json().catch(() => null) as ErrorBody | null
    throw new ApiError(
      response.status,
      body?.error?.message ?? 'Une erreur est survenue.',
      body?.error?.code,
    )
  }

  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}
