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

/**
 * French messages for the stable API error codes. The API returns English
 * technical messages; the interface shows these instead.
 */
export const errorMessages: Record<string, string> = {
  VALIDATION_ERROR: 'La requête envoyée au Manager est invalide.',
  AUTH_UNAVAILABLE:
    "Authentification indisponible : la base de données du Manager n'est pas accessible.",
  INVALID_CREDENTIALS: 'Identifiant ou mot de passe incorrect.',
  SESSION_NOT_FOUND: 'Session expirée. Reconnectez-vous.',
  CROSS_ORIGIN_REQUEST: 'Requête refusée : origine non autorisée.',
  RATE_LIMITED: 'Trop de tentatives de connexion. Réessayez dans une minute.',
  NOT_FOUND: 'Élément introuvable.',
  METHOD_NOT_ALLOWED: 'Opération non autorisée.',
  METRICS_UNAVAILABLE: 'Métriques momentanément indisponibles.',
  TOO_MANY_POINTS: 'Trop de points pour cette période : choisissez une période plus courte.',
  SETTINGS_UPDATE_FAILED: "Les paramètres n'ont pas pu être enregistrés.",
  EVENTS_UNAVAILABLE: 'Événements momentanément indisponibles.',
  LOGOUT_FAILED: 'La déconnexion a échoué.',
  PASSWORD_CHANGE_FAILED: "Le mot de passe n'a pas pu être modifié.",
  FORBIDDEN: 'Votre rôle ne permet pas cette action.',
  PASSWORD_CHANGE_REQUIRED: 'Choisissez votre mot de passe pour continuer.',
  USERNAME_TAKEN: 'Cet identifiant est déjà utilisé.',
  LAST_ADMIN:
    'Le dernier administrateur actif ne peut être ni supprimé, ni rétrogradé, ni désactivé.',
  USERS_UNAVAILABLE: 'Comptes momentanément indisponibles.',
  ACCOUNT_UPDATE_FAILED: "Le compte n'a pas pu être modifié.",
  INTERNAL_ERROR: 'Erreur interne du Manager.',
}

/** True when the error means the session is missing, expired or revoked. */
export function isUnauthenticated(error: unknown): boolean {
  return error instanceof ApiError && error.status === 401 && error.code === 'SESSION_NOT_FOUND'
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
    const body = (await response.json().catch(() => null)) as ErrorBody | null
    const code = body?.error?.code
    throw new ApiError(
      response.status,
      (code && errorMessages[code]) ?? 'Une erreur est survenue.',
      code,
    )
  }

  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}
