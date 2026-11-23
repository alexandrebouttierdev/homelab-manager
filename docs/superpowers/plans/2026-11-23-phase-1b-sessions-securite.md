# Phase 1B — Sessions, verrouillage, politique de sécurité et audit : plan d'implémentation

> **Pour les agents :** sous-skill requis : `superpowers:executing-plans` ou `superpowers:subagent-driven-development`. Chaque tâche suit le cycle test qui échoue → implémentation → `scripts/verify.sh` → commit.

**Objectif :** livrer `SPEC_ADMINISTRATION.md` §§ 3.4 et 3.5 hors second facteur (1C) et jetons d'API (phase 8) : sessions consultables et révocables, expiration par inactivité, verrouillage de compte, réseaux autorisés, journal d'audit consultable et exportable, audit des refus (§ 2.2, question Q-3).

**Architecture :** la politique (inactivité, réseaux) est un objet du domaine, gardée en mémoire par un `SecurityUsecase` chargé au démarrage et mis à jour à chaque modification ; l'`AuthUsecase` la lit pour l'inactivité et gère le verrouillage à partir d'un compteur persistant. Un middleware applique la liste de réseaux à toute l'interface sauf `/health`. Les refus d'autorisation et les refus métier (`LAST_ADMIN`) passent par le port `AuditRecorder` du module M. Le journal d'audit est lu par un `AuditUsecase` borné.

**Prérequis :** phase 1A et module M livrés.

**Écart assumé au format habituel :** ce plan est exécuté dans la foulée par son auteur ; il fixe la conception, les interfaces, les règles et les cas de test de chaque tâche, et le code est écrit test d'abord pendant l'exécution plutôt que recopié ici.

## Décisions de conception

| Sujet | Choix | Motif |
|---|---|---|
| Inactivité | `idle_timeout_minutes` réglable de 5 à 720 (défaut 60) ; une session inactive est révoquée à la requête suivante | Spec § 3.4 ; 720 min = durée maximale de 12 h |
| Dernière activité | `last_seen_at` écrit au plus une fois par minute et par session | Limiter les écritures SQLite (connexion unique) |
| Rafraîchissement automatique | Les requêtes périodiques de l'interface comptent comme de l'activité : un onglet ouvert garde sa session | Limite documentée ; distinguer l'activité humaine exigerait un mécanisme côté client non spécifié |
| Verrouillage | Constantes du domaine : 5 échecs consécutifs → 15 min ; compteur remis à zéro au verrouillage et à la connexion réussie | Valeurs fixées par la spec |
| Réponse d'un compte verrouillé | `401 INVALID_CREDENTIALS`, vérification Argon2id faite quand même | Pas d'énumération ni d'oracle de verrouillage |
| Réseaux autorisés | Liste CIDR (32 au plus) ; vide = tous ; appliquée à `/api` et à l'interface, pas à `/health` ; `403 NETWORK_NOT_ALLOWED` | Spec § 3.4 ; `/health` reste joignable par la supervision locale |
| Auto-exclusion | Refus `409 POLICY_EXCLUDES_CURRENT_ADDRESS` si l'adresse de l'administrateur qui modifie n'est pas couverte | Spec § 3.4 |
| Adresse cliente | `RemoteAddr`, comme la limite par IP existante ; derrière un proxy inverse, c'est l'adresse du proxy | Limite documentée (aucun en-tête `X-Forwarded-For` n'est cru) |
| Révocation | Chacun révoque ses sessions ; un administrateur (`PermManageUsers`) révoque toutes les sessions | Spec § 3.5 |
| Audit consulté | Rôle `admin` (`PermViewAudit`) ; filtres acteur, type de cible, action, résultat, période ; 200 lignes par défaut, 1 000 au plus ; export CSV ou JSON de 10 000 lignes au plus | Spec § 3.5 ; bornes |
| Audit des refus | `403 FORBIDDEN` (cible `route`, `METHODE chemin`, action `access`) et `LAST_ADMIN` (cible `user`) avec `outcome = denied` ; verrouillage (`user`, `lock`) | Spec § 2.2, Q-3 |
| Hors périmètre | 2FA obligatoire pour les administrateurs (1C), jetons d'API (phase 8), réinitialisation par un administrateur (Q-1), changement forcé de `admin`/`admin` (Q-2), purge du journal d'audit | Non spécifiés ici ou décisions ouvertes |

## Données — migration `004_sessions_security.sql`

```sql
ALTER TABLE sessions ADD COLUMN last_seen_at DATETIME;
ALTER TABLE sessions ADD COLUMN auth_method TEXT NOT NULL DEFAULT 'password';
UPDATE sessions SET last_seen_at = created_at;
CREATE TABLE security_policies (singleton, idle_timeout_minutes CHECK 5..720, allowed_networks JSON, updated_at);
CREATE TABLE login_failures (id = UUID du compte, ON DELETE CASCADE, consecutive_failures, last_failure_at, locked_until);
CREATE INDEX audit_logs_created_idx ON audit_logs (created_at DESC);
```

## Tâches

### Tâche 1 : domaine
- `domain/security.go` : `SecurityPolicy{IdleTimeoutMinutes int; AllowedNetworks []netip.Prefix}`, `DefaultSecurityPolicy()`, `(SecurityPolicy).Allows(addr string) bool`, `(SecurityPolicy).IdleTimeout()`, `ParseNetworks([]string) ([]netip.Prefix, error)`, `NewSecurityPolicy(minutes int, networks []string) (SecurityPolicy, error)` ; constantes `MinIdleTimeoutMinutes = 5`, `MaxIdleTimeoutMinutes = 720`, `MaxAllowedNetworks = 32`, `MaxConsecutiveFailures = 5`, `LockoutDuration = 15 min` ; `LoginFailures{Count int; LockedUntil *time.Time}` avec `Locked(now)` et `AfterFailure(now) (LoginFailures, bool)` ; erreurs `ErrInvalidSecurityPolicy`, `ErrPolicyExcludesCurrentAddress`, `ErrForbidden`.
- `domain/auth.go` : `Session` enrichie (`IPAddress`, `UserAgent`, `AuthMethod`, `CreatedAt`, `LastSeenAt`, `Username`).
- `domain/audit.go` : `AuditRecord`, `AuditFilter` (bornes), outcome constants.
- `domain/role.go` : `PermViewAudit` (admin).
- Tests : bornes de la politique, CIDR invalides, IPv4/IPv6, liste vide, adresse non analysable refusée dès qu'une liste existe, transitions de verrouillage avec horloge fixe.

### Tâche 2 : persistance
- Migration `004`, schémas Ent `SecurityPolicy`, `LoginFailure`, champs de `Session` ; régénération (`GOTOOLCHAIN=go1.26.10`).
- `AuthRepository` : `Create` écrit `auth_method = password` et `last_seen_at` ; `Touch(ctx, id, at)` ; `ListActive(ctx, userID *uuid.UUID, now, limit)` ; `RevokeByID(ctx, id) (domain.Session, error)` ; `RevokeAllForUserExcept(ctx, userID, keep) (int, error)` ; `FindActiveByTokenHash` renvoie la session complète.
- `SecurityRepository` : `SecurityPolicy(ctx)` (défaut si absente), `SetSecurityPolicy(ctx, policy, audit)` transactionnel ; `LoginFailures(ctx, userID)`, `SaveLoginFailures(ctx, userID, failures)`, `ClearLoginFailures(ctx, userID)`.
- `AuditRepository.ListAudit(ctx, filter)` (tri décroissant, filtres liés en paramètres).
- Tests SQLite réels : défaut puis enregistrement de la politique avec audit, compteurs d'échec, cascade à la suppression d'un compte, liste et révocation de sessions, filtres de l'audit, migration `004` sur une base `003`.

### Tâche 3 : usecases
- `SecurityUsecase` : `Load`, `Policy`, `AllowsAddress`, `UpdatePolicy(ctx, actor, currentAddress, minutes, networks, requestID)`.
- `AuthUsecase` : verrouillage (lecture avant vérification, échec compté, verrouillage audité, remise à zéro à la réussite) ; inactivité (révocation, `Touch` limité à une fois par minute) ; `ListSessions`, `RevokeSession`, `RevokeOtherSessions` avec contrôle propriétaire/administrateur et audit.
- `AccountUsecase` : refus `LAST_ADMIN` audité `denied`.
- `AuditUsecase` : `List`, `Export`, `RecordDenied`.
- Tests avec horloge injectée et faux objets.

### Tâche 4 : HTTP et documentation du contrat
- Middlewares : `allowedNetworks` (tout sauf `/health`), `requirePermission` audite les refus, `requireSession` place la session dans le contexte.
- Routes : `GET /api/v1/auth/sessions[?scope=all]`, `DELETE /api/v1/auth/sessions/{sessionID}`, `POST /api/v1/auth/sessions/revoke-others`, `GET|PUT /api/v1/security/policy`, `GET /api/v1/audit`, `GET /api/v1/audit/export?format=csv|json`.
- `API_CONTRACTS.md`, `SECURITY.md`, `DATA_MODEL.md`.
- Tests : validation (chemins, filtres, dates, formats), parcours complet sur SQLite réel (verrouillage, inactivité, révocations, politique et auto-exclusion, refus réseau, audit des refus, export).

### Tâche 5 : interface
- Paramètres : carte « Sessions actives » (ses sessions, révoquer, « toutes sauf celle-ci »).
- « Utilisateurs & accès » : onglets Comptes, Sessions (toutes), Politique (inactivité, réseaux), Journal d'audit (filtres, export CSV/JSON).
- Clients d'API, schémas Zod (politique), fonctions pures (libellés de navigateur, lignes de session et d'audit), tests.

### Tâche 6 : état et vérification
- `README.md` (réseaux autorisés et récupération si l'on s'est exclu), `PRODUCT_SPEC.md`, `SPEC_ADMINISTRATION.md`, `TESTING.md`, feuille de route.
- `scripts/verify.sh`, parcours réel dans Chromium contre le binaire.
