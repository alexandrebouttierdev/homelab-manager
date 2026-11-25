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
  HOSTNAME_MISMATCH: 'Le nom saisi ne correspond pas au nom de la machine.',
  POWER_ACTIONS_DISABLED:
    "Le redémarrage et l'extinction sont désactivés dans la configuration du Manager.",
  ACTION_IN_PROGRESS: 'Une action est déjà en cours sur cette cible.',
  POWER_ACTION_REFUSED:
    "La machine a refusé : règle polkit absente, ou une opération en cours (sauvegarde, mise à jour) bloque l'arrêt.",
  POWER_ACTION_FAILED: "La demande n'a pas pu être transmise à la machine.",
  NETWORK_NOT_ALLOWED: "Accès refusé : cette adresse n'est pas dans les réseaux autorisés.",
  POLICY_EXCLUDES_CURRENT_ADDRESS:
    'Refusé : cette politique exclurait votre propre adresse. Ajoutez un réseau qui la contient.',
  SESSIONS_UNAVAILABLE: 'Sessions momentanément indisponibles.',
  SESSION_REVOKE_FAILED: "La session n'a pas pu être révoquée.",
  SECURITY_POLICY_UPDATE_FAILED: "La politique de sécurité n'a pas pu être enregistrée.",
  AUDIT_UNAVAILABLE: "Journal d'audit momentanément indisponible.",
  JOURNAL_UNAVAILABLE:
    'Journal système indisponible : journalctl est introuvable sur la machine du Manager.',
  JOURNAL_READ_FAILED: "Le journal système n'a pas pu être lu.",
  SYSTEMD_UNAVAILABLE:
    "Services système indisponibles : systemd n'est pas joignable sur le bus système de la machine.",
  SYSTEMD_READ_FAILED: "L'unité systemd n'a pas pu être lue.",
  TOKEN_INVALID: "Jeton d'API invalide, expiré ou révoqué.",
  TOKEN_SCOPE_DENIED: "Ce jeton d'API ne permet pas cette requête.",
  TOO_MANY_TOKENS: "Nombre maximal de jetons d'API atteint (20 par compte).",
  TOKENS_UNAVAILABLE:
    "Jetons d'API indisponibles : la base de données du Manager n'est pas accessible.",
  TOKENS_FAILED: "Les jetons d'API n'ont pas pu être lus ou modifiés.",
  CRON_UNAVAILABLE: 'Tâches planifiées indisponibles.',
  CRON_READ_FAILED: "Les tâches planifiées n'ont pas pu être lues.",
  CRON_EXPRESSION_INVALID:
    'Expression Cron invalide : 5 champs (minute, heure, jour, mois, jour de semaine) ou une macro (@daily…).',
  BACKUPS_UNAVAILABLE:
    "Sauvegardes indisponibles : la base de données du Manager n'est pas accessible.",
  BACKUPS_FAILED: "Les sauvegardes n'ont pas pu être lues ou lancées.",
  BACKUP_NOT_CONFIGURED: "Aucun dépôt de sauvegarde n'est configuré.",
  BACKUP_NOT_INITIALIZED: "Le dépôt n'est pas encore initialisé.",
  BACKUP_ALREADY_INITIALIZED: 'Le dépôt est déjà initialisé.',
  BACKUP_IN_PROGRESS: 'Une opération de sauvegarde est déjà en cours.',
  BACKUP_COPY_NOT_CONFIGURED: "Aucun dépôt de copie n'est configuré.",
  BACKUP_COPY_NOT_INITIALIZED: "Le dépôt de copie n'est ni créé ni rattaché.",
  BACKUP_COPY_ALREADY_INITIALIZED: 'Le dépôt de copie est déjà prêt.',
  BACKUP_SSH_KEY_MISSING:
    "Un dépôt SFTP a besoin de la clé SSH du Manager : générez-la d'abord, puis installez-la sur le serveur.",
  UNIT_NOT_MANAGEABLE:
    "Cette unité n'est pas dans la liste des unités gérables de la configuration du Manager.",
  UNIT_ACTION_RATE_LIMITED: "Trop d'actions sur les services en une minute. Réessayez plus tard.",
  UNIT_ACTIONS_UNAVAILABLE:
    "Actions sur les services indisponibles : la base de données du Manager n'est pas accessible.",
  UNIT_ACTION_FAILED: "L'action n'a pas pu être demandée.",
  TOKEN_SCOPE_NOT_ALLOWED: "Votre rôle ne permet pas cette portée d'action.",
  TOKEN_ACTION_NOT_AUDITED:
    "L'action n'a pas pu être inscrite au journal d'audit : elle n'a pas été faite.",
  CONTAINER_ACTIONS_DISABLED:
    'Actions sur les conteneurs désactivées dans la configuration du Manager (docker_actions_enabled).',
  CONTAINER_REMOVED: "Ce conteneur n'existe plus sur l'hôte Docker.",
  CONTAINER_ACTION_RATE_LIMITED:
    "Trop d'actions sur les conteneurs en une minute. Réessayez plus tard.",
  CONTAINERS_UNAVAILABLE:
    "Journaux et actions des conteneurs indisponibles : la base de données du Manager n'est pas accessible.",
  CONTAINER_ACTION_FAILED: "L'action n'a pas pu être demandée.",
  DOCKER_UNAVAILABLE: "Le journal n'a pas pu être lu : le proxy Docker ou Docker est injoignable.",
  ALERTS_UNAVAILABLE: "Alertes indisponibles : la base de données du Manager n'est pas accessible.",
  ALERTS_FAILED: "Les alertes n'ont pas pu être lues ou enregistrées.",
  ALERT_ALREADY_RESOLVED: "L'alerte est déjà résolue.",
  DEFAULT_RULE: 'Une règle par défaut peut être désactivée, pas supprimée.',
  TOO_MANY_RULES: 'Nombre maximal de règles atteint (200).',
  TOO_MANY_SILENCES: 'Nombre maximal de silences en vigueur atteint (100).',
  TOO_MANY_CHANNELS: 'Nombre maximal de canaux de notification atteint (20).',
  SECRETS_UNAVAILABLE:
    'Secrets indisponibles : la clé du coffre est absente. Les canaux sans secret restent utilisables.',
  NOTIFICATION_FAILED: "L'envoi de test a échoué : voir la dernière erreur du canal.",
  SCAN_IN_PROGRESS: 'Une analyse est déjà en cours.',
  SCAN_RATE_LIMITED: 'Une analyse manuelle a été lancée il y a moins de 5 minutes.',
  VULNERABILITIES_UNAVAILABLE:
    "Vulnérabilités indisponibles : la base de données du Manager n'est pas accessible.",
  VULNERABILITIES_FAILED: "Les vulnérabilités n'ont pas pu être lues ou l'analyse lancée.",
  OWN_PASSWORD_RESET: 'Changez votre propre mot de passe dans Paramètres.',
  INVALID_SECOND_FACTOR: 'Code incorrect ou clé refusée.',
  LOGIN_CHALLENGE_EXPIRED: 'Délai dépassé : reconnectez-vous avec votre mot de passe.',
  PASSKEY_CEREMONY_EXPIRED: 'Délai dépassé : relancez la connexion par passkey.',
  PASSKEY_LOGIN_DISABLED:
    "La connexion par passkey seule n'est pas autorisée : connectez-vous avec votre mot de passe.",
  INVALID_PASSKEY: 'Cette passkey ne permet pas de se connecter.',
  PASSKEY_PASSWORD_CHANGE:
    'Ce compte doit changer son mot de passe : connectez-vous avec lui une première fois.',
  SECOND_FACTOR_REQUIRED: 'Activez la double authentification pour continuer.',
  SECOND_FACTOR_UNAVAILABLE:
    'Double authentification par application indisponible : la clé du coffre est absente. Utilisez un code de secours.',
  SECOND_FACTOR_ALREADY_ENABLED: 'La double authentification est déjà activée.',
  SECOND_FACTOR_NOT_ENABLED: "La double authentification n'est pas activée.",
  NO_PENDING_ENROLLMENT: "Recommencez l'activation de la double authentification.",
  SECOND_FACTOR_NOT_ENROLLED: "Activez d'abord votre propre double authentification.",
  SECOND_FACTOR_UPDATE_FAILED: "La double authentification n'a pas pu être modifiée.",
  WEBAUTHN_UNAVAILABLE:
    "Clés de sécurité indisponibles : le domaine WebAuthn du Manager n'est pas configuré.",
  WEBAUTHN_KEY_NOT_FOUND: "Cette clé de sécurité n'existe pas ou plus.",
  WEBAUTHN_KEY_EXISTS: 'Cette clé de sécurité est déjà enregistrée.',
  TOO_MANY_WEBAUTHN_KEYS: 'Nombre maximal de clés de sécurité atteint (10 par compte).',
  INVALID_WEBAUTHN_RESPONSE: 'La réponse de la clé de sécurité a été refusée.',
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
