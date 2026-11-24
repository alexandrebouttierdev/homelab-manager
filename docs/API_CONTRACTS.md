# Contrats API — Homelab Manager

Version 2.1 — 22 novembre 2026

Ce document est le contrat exact de l'API implémentée. Toute différence entre ce document et le code est un défaut : corriger l'un ou l'autre dans la même modification. Les éléments non encore implémentés sont regroupés en fin de document (§ 9) et ne doivent pas être présentés comme disponibles.

## 1. Principes communs

- API REST JSON UTF-8 sous `/api/v1`, servie sur la même origine que l'interface.
- DTO explicites définis dans `internal/delivery/http` ; jamais d'entité Ent ni de réponse Docker brute.
- Identifiants : UUID générés par le Manager ; l'ID Docker (hexadécimal) n'est qu'un identifiant externe de conteneur.
- Horodatages émis en RFC 3339 UTC (`2026-11-21T20:58:00Z`, fraction de seconde possible).
- Une valeur inconnue est `null` ou absente selon le DTO, jamais remplacée par zéro. Zéro est une mesure réelle.
- Corps de requête limité à 1 MiB, JSON strict : champ inconnu ou second document JSON → `400 VALIDATION_ERROR`.
- Toutes les routes sauf `/health`, `/api/v1/health`, `/api/v1/auth/login`, `/api/v1/auth/login/second-factor` et `/api/v1/auth/session` exigent une session.
- Les messages d'erreur de l'API sont techniques et en anglais ; l'interface affiche ses propres messages, en français, à partir du `code`.

## 2. Routes

| Méthode et chemin | Session | Réponse |
|---|---|---|
| `GET /health` | non | Processus HTTP vivant (§ 4.1) |
| `GET /api/v1/health` | non | Santé détaillée (§ 4.1) |
| `POST /api/v1/auth/login` | non | Ouvre une session, ou un défi de second facteur (§ 3) |
| `POST /api/v1/auth/login/second-factor` | défi | Termine une connexion avec un second facteur (§ 3.5) |
| `GET /api/v1/auth/session` | cookie | Utilisateur courant (§ 3) |
| `GET /api/v1/auth/second-factor` | oui | État de son second facteur (§ 3.5) |
| `POST /api/v1/auth/second-factor/totp` | oui | Commence l'enrôlement TOTP (§ 3.5) |
| `POST /api/v1/auth/second-factor/totp/confirm` | oui | Confirme le TOTP, renvoie les codes de secours (§ 3.5) |
| `POST /api/v1/auth/second-factor/recovery-codes` | oui | Régénère les codes de secours (§ 3.5) |
| `POST /api/v1/auth/second-factor/disable` | oui | Désactive son second facteur (§ 3.5) |
| `POST /api/v1/auth/logout` | oui | Révoque la session |
| `POST /api/v1/auth/password` | oui | Change le mot de passe |
| `GET /api/v1/auth/sessions` | oui | Ses sessions actives ; `?scope=all` : toutes (admin) (§ 3.2) |
| `DELETE /api/v1/auth/sessions/{sessionID}` | oui | Révoque une session (§ 3.2) |
| `POST /api/v1/auth/sessions/revoke-others` | oui | Révoque ses autres sessions (§ 3.2) |
| `GET /api/v1/security/policy` | admin | Politique de sécurité (§ 3.3) |
| `PUT /api/v1/security/policy` | admin | Modifie la politique (§ 3.3) |
| `GET /api/v1/audit` | admin | Journal d'audit filtré (§ 3.4) |
| `GET /api/v1/audit/export` | admin | Export CSV ou JSON du journal (§ 3.4) |
| `GET /api/v1/dashboard` | oui | Synthèse (§ 4.2) |
| `GET /api/v1/host` | oui | Identité de l'hôte (§ 4.3) |
| `GET /api/v1/host/metrics/current` | oui | Dernier cycle hôte (§ 5.2) |
| `GET /api/v1/host/metrics/{metricName}` | oui | Historique brut ou agrégé (§ 5.3) |
| `GET /api/v1/host/temperatures` | oui | Températures du dernier cycle (§ 5.2) |
| `GET /api/v1/containers` | oui | Inventaire (§ 6.1) |
| `GET /api/v1/containers/{containerID}` | oui | Détail (§ 6.1) |
| `GET /api/v1/containers/{containerID}/metrics/current` | oui | Dernier cycle du conteneur (§ 5.2) |
| `GET /api/v1/containers/{containerID}/metrics/{metricName}` | oui | Historique brut ou agrégé (§ 5.3) |
| `GET /api/v1/containers/{containerID}/events` | oui | Événements du conteneur (§ 6.2) |
| `GET /api/v1/events` | oui | Tous les événements (§ 6.2) |
| `GET /api/v1/settings` | oui | Paramètres effectifs (§ 4.4) |
| `PUT /api/v1/settings/retention` | admin | Modifie la rétention (§ 4.4) |
| `GET /api/v1/users` | admin | Comptes (§ 3.1) |
| `POST /api/v1/users` | admin | Crée un compte (§ 3.1) |
| `PATCH /api/v1/users/{userID}` | admin | Modifie un compte (§ 3.1) |
| `DELETE /api/v1/users/{userID}` | admin | Supprime un compte (§ 3.1) |
| `POST /api/v1/users/{userID}/password` | admin | Réinitialise le mot de passe d'un compte (§ 3.1) |
| `GET /api/v1/machine/power` | admin | Actions d'alimentation disponibles (§ 4.5) |
| `POST /api/v1/machine/power/{action}` | admin | Redémarre ou éteint la machine (§ 4.5) |
| `GET /api/v1/journal/entries` | oui | Entrées du journal systemd, filtrées et paginées (§ 4.6) |
| `GET /api/v1/journal/entry` | oui | Tous les champs d'une entrée, masqués (§ 4.6) |
| `GET /api/v1/journal/boots` | oui | Démarrages enregistrés (§ 4.6) |
| `GET /api/v1/journal/units` | oui | Unités présentes dans le journal (§ 4.6) |
| `GET /api/v1/journal/summary` | oui | Volume sur 24 h par priorité, occupation disque (§ 4.6) |
| `GET /api/v1/journal/export` | oui | Export `.log` ou `.json` (§ 4.6) |
| `GET /api/v1/system/units` | oui | Inventaire des services, timers et sockets systemd (§ 4.7) |
| `GET /api/v1/system/units/{unitName}` | oui | Détail d'une unité et extrait de son journal (§ 4.7) |
| `POST /api/v1/system/units/{unitName}/actions` | `operator` | Démarre, arrête, redémarre ou recharge une unité de la liste blanche (§ 4.7) |
| `GET /api/v1/system/units/{unitName}/actions/last` | oui | Dernière action demandée sur l'unité (§ 4.7) |
| `GET /api/v1/alerts` | oui | Alertes ouvertes et historique (§ 4.8) |
| `GET /api/v1/alerts/summary` | oui | Indicateurs des alertes (§ 4.8) |
| `GET /api/v1/alerts/{alertID}` | oui | Une alerte (§ 4.8) |
| `POST /api/v1/alerts/{alertID}/acknowledge` | `operator` | Acquitte une alerte ouverte (§ 4.8) |
| `GET /api/v1/alert-rules` | oui | Règles d'alerte et catalogue des valeurs acceptées (§ 4.8) |
| `GET /api/v1/alert-rules/{ruleID}` | oui | Une règle (§ 4.8) |
| `POST /api/v1/alert-rules` | admin | Crée une règle (§ 4.8) |
| `PUT /api/v1/alert-rules/{ruleID}` | admin | Remplace une règle (§ 4.8) |
| `DELETE /api/v1/alert-rules/{ruleID}` | admin | Supprime une règle qui n'est pas une règle par défaut (§ 4.8) |
| `GET /api/v1/alert-silences` | oui | Silences en vigueur et récents (§ 4.8) |
| `POST /api/v1/alert-silences` | `operator` | Pose un silence (§ 4.8) |
| `DELETE /api/v1/alert-silences/{silenceID}` | `operator` | Lève un silence avant son terme (§ 4.8) |
| `GET /api/v1/notification-channels` | admin | Canaux de notification, sans leurs secrets (§ 4.8) |
| `POST /api/v1/notification-channels` | admin | Crée un canal (§ 4.8) |
| `PUT /api/v1/notification-channels/{channelID}` | admin | Remplace un canal (§ 4.8) |
| `DELETE /api/v1/notification-channels/{channelID}` | admin | Supprime un canal (§ 4.8) |
| `POST /api/v1/notification-channels/{channelID}/test` | admin | Envoie un message de test (§ 4.8) |
| `GET /api/v1/vulnerabilities/summary` | oui | Synthèse des vulnérabilités des paquets (§ 4.9) |
| `PUT /api/v1/settings/vulnerability-scans` | admin | Modifie l'intervalle des scans de vulnérabilités (§ 4.9) |
| `GET /api/v1/vulnerabilities` | oui | Constats filtrés et paginés (§ 4.9) |
| `GET /api/v1/vulnerabilities/{findingID}` | oui | Un constat (§ 4.9) |
| `GET /api/v1/vulnerabilities/scans` | oui | Derniers scans (§ 4.9) |
| `POST /api/v1/vulnerabilities/scans` | `operator` | Lance un scan (§ 4.9) |

Toutes les routes de supervision sont en lecture seule ; les seules écritures sont l'authentification, `PUT /api/v1/settings/retention`, la gestion des comptes, les demandes d'alimentation de la machine, les règles d'alerte, l'acquittement d'une alerte, les silences, les canaux de notification et le lancement d'un scan de vulnérabilités. La colonne « Session » indique `admin` pour les routes réservées au rôle administrateur, `operator` pour celles ouvertes aux opérateurs et administrateurs (§ 3.1). Une route inconnue sous `/api` répond `404 NOT_FOUND`, une méthode non prévue `405 METHOD_NOT_ALLOWED`, toujours au format d'erreur du § 8. Le journal système et les unités systemd ne sont lus qu'en lecture seule. Il n'existe aucune route d'Agent, d'hôte distant, de journaux de conteneurs, ni d'action Docker (création, suppression, démarrage, arrêt, restart, exec).

### Paramètres de chemin

- `containerID` : UUID Manager (`8-4-4-4-12` hexadécimal minuscule) ou ID Docker de 12 à 64 caractères hexadécimaux minuscules. Sinon `400 VALIDATION_ERROR` ; inconnu → `404 NOT_FOUND`.
- `userID` : UUID Manager en minuscules (`8-4-4-4-12`). Sinon `400 VALIDATION_ERROR` ; inconnu → `404 NOT_FOUND`.
- `action` : `reboot` ou `poweroff`. Sinon `400 VALIDATION_ERROR`.
- `alertID`, `ruleID` : UUID en minuscules (`8-4-4-4-12`). Sinon `400 VALIDATION_ERROR` ; inconnu → `404 NOT_FOUND`.
- `unitName` : nom d'unité systemd, éventuellement encodé (`getty%40tty1.service`), motif `^[A-Za-z0-9@._:\-]{1,255}\.(service|timer|socket)$`. Sinon `400 VALIDATION_ERROR` ; absent du dernier inventaire → `404 NOT_FOUND`.
- `metricName` : 64 caractères au plus, motif `^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$` (ex. `cpu.percent`). Sinon `400 VALIDATION_ERROR`. Un nom valide mais jamais collecté renvoie une liste vide.

## 3. Authentification et sessions

**Connexion** — `POST /api/v1/auth/login` avec `{ "username": "...", "password": "..." }`.

- `username` : espaces de début et fin retirés, 1 à 255 octets. `password` : 1 à 1 024 octets. Aucune longueur minimale à la connexion (le compte par défaut est `admin` / `admin`).
- Succès `200` : `{ "user": { "id": "uuid", "username": "admin", "displayName": "admin", "role": "admin", "mustChangePassword": false, "mustEnrollSecondFactor": false } }` et cookie de session. Un compte désactivé reçoit `401 INVALID_CREDENTIALS`, comme un mot de passe faux.
- Compte avec un second facteur : `200 { "secondFactorRequired": true, "methods": ["totp", "recovery"], "expiresAt": "…" }` et cookie de défi, **sans session** ; la connexion se termine par `POST /api/v1/auth/login/second-factor` (§ 3.5). `mustEnrollSecondFactor` (§ 3.3) : la politique exige un second facteur des administrateurs et ce compte n'en a pas ; toutes les routes authentifiées sauf la session, la déconnexion, le mot de passe et `/auth/second-factor*` répondent alors `403 SECOND_FACTOR_REQUIRED`.
- Échec : `401 INVALID_CREDENTIALS`, identique que l'utilisateur existe ou non ; le temps de réponse est équivalent (une vérification Argon2id est faite dans les deux cas).
- Limitation : 10 tentatives par minute et par adresse IP source, au-delà `429 RATE_LIMITED` (fenêtre fixe d'une minute, au plus 10 000 adresses suivies). Le compteur est partagé avec `POST /auth/login/second-factor` : une connexion en deux étapes compte deux tentatives.
- Contrôle d'origine identique aux mutations (ci-dessous).

**Cookie** — nom `homelab_session`, `HttpOnly`, `SameSite=Lax`, `Path=/`, `Secure` si TLS est activé ou `HOMELAB_SESSION_SECURE=true`. Durée de vie fixe de 12 heures, sans prolongation. Seul le SHA-256 du jeton (32 octets aléatoires) est stocké.

**Session** — `GET /api/v1/auth/session` : `200 { "user": … }` (même objet que la connexion), sinon `401 SESSION_NOT_FOUND`, y compris pour un compte désactivé ou supprimé.

**Déconnexion** — `POST /api/v1/auth/logout` : révoque la session, efface le cookie, `200 { "loggedOut": true }`.

**Changement de mot de passe** — `POST /api/v1/auth/password` avec `{ "currentPassword": "...", "newPassword": "..." }`.

- `newPassword` : 12 à 1 024 octets ; `currentPassword` : 1 024 octets au plus. Sinon `400 VALIDATION_ERROR`.
- Mot de passe actuel faux : `401 INVALID_CREDENTIALS`.
- Succès `200 { "changed": true }`. **Toutes les sessions de l'utilisateur sont révoquées, y compris la session courante** : l'interface renvoie vers la connexion.

**Contrôle d'origine (CSRF)** — appliqué à la connexion et à toutes les routes authentifiées : si l'en-tête `Origin` est présent, son hôte doit être égal à l'en-tête `Host` ; une requête `Sec-Fetch-Site: cross-site` est refusée. Refus : `403 CROSS_ORIGIN_REQUEST`. Combiné à `SameSite=Lax`, il protège les mutations par cookie.

**Base indisponible** — si la base SQLite n'a pas pu être ouverte au démarrage, toutes les routes qui exigent une session et la connexion répondent `503 AUTH_UNAVAILABLE`.

### 3.1 Comptes et rôles

Comptes de Homelab Manager uniquement : aucun compte Linux n'est lu ni modifié.

**Rôles** — `viewer` : consultation et son propre mot de passe. `operator` : idem, plus l'acquittement des alertes et les silences (permission `alerts:operate`), le lancement d'un scan de vulnérabilités (`vulnerabilities:scan`) et les actions sur les unités de la liste blanche (`services:manage`, § 4.7) ; sauvegardes dans une phase suivante. `admin` : idem, plus `PUT /api/v1/settings/retention`, les routes `/api/v1/users` et la gestion des règles d'alerte (`alerts:manage`). Un rôle insuffisant reçoit `403 FORBIDDEN`.

**Mot de passe initial** — un compte créé par un administrateur a `mustChangePassword: true`. Tant qu'il ne l'a pas changé (`POST /api/v1/auth/password`), toutes les routes authentifiées sauf `/auth/session`, `/auth/logout` et `/auth/password` répondent `403 PASSWORD_CHANGE_REQUIRED`.

**Compte** :

```json
{
  "id": "uuid", "username": "alice", "displayName": "Alice", "role": "viewer",
  "disabled": false, "mustChangePassword": true,
  "lastLoginAt": null, "lastLoginIp": null, "createdAt": "2026-11-23T10:00:00Z"
}
```

- `GET /api/v1/users` : `200 { "users": [Compte] }`, triés par identifiant, 500 au plus.
- `POST /api/v1/users` avec `{ "username", "displayName", "role", "password" }` : `username` selon `^[a-z0-9._-]{1,64}$` ; `displayName` 128 octets au plus, sans caractère de contrôle, espaces de bord retirés, identifiant par défaut ; `role` ∈ `viewer`, `operator`, `admin` ; `password` 12 à 1 024 octets. Succès `201 { "user": Compte }`. Identifiant déjà pris : `409 USERNAME_TAKEN`.
- `PATCH /api/v1/users/{userID}` avec au moins un champ parmi `{ "displayName", "role", "disabled" }` : succès `200 { "user": Compte }`. Désactiver un compte révoque ses sessions.
- `DELETE /api/v1/users/{userID}` : `200 { "deleted": true }`. Les sessions du compte sont supprimées ; ses lignes d'audit sont conservées.
- `POST /api/v1/users/{userID}/password` avec `{ "password": "..." }` (12 à 1 024 octets) : `200 { "user": Compte }` avec `mustChangePassword: true`. Le titulaire devra changer ce mot de passe à sa connexion suivante ; ses sessions sont révoquées et un verrouillage en cours est levé. Audité (`user`, UUID, `reset_password`). Sur son propre compte : `409 OWN_PASSWORD_RESET` (utiliser `POST /api/v1/auth/password`).
- Compte initial : créé sans `HOMELAB_ADMIN_PASSWORD`, `admin` / `admin` a `mustChangePassword: true` ; au démarrage, un compte existant qui accepte encore ce mot de passe reçoit la même obligation.
- Le dernier administrateur actif ne peut être ni rétrogradé, ni désactivé, ni supprimé : `409 LAST_ADMIN`.
- Chaque modification réussie écrit une ligne d'audit (`user`, UUID du compte, `create` / `update` / `delete`) dans la même transaction.

### 3.2 Sessions

- **Verrouillage** — 5 mots de passe faux consécutifs verrouillent le compte 15 minutes, en plus de la limite par adresse. Un compte verrouillé reçoit `401 INVALID_CREDENTIALS`, comme un mot de passe faux, après la même vérification Argon2id. Une connexion réussie remet le compteur à zéro ; le verrouillage est audité (`system`, `user`, `lock`).
- **Inactivité** — une session inactive depuis plus de `idleTimeoutMinutes` (§ 3.3) est révoquée à sa requête suivante (`401 SESSION_NOT_FOUND`), en plus de la durée maximale de 12 h. La dernière activité est enregistrée au plus une fois par minute ; les requêtes périodiques de l'interface comptent comme de l'activité.
- `GET /api/v1/auth/sessions` : `200 { "sessions": [Session] }`, sessions actives du compte, les plus récemment actives d'abord, 500 au plus. `?scope=all` : sessions de tous les comptes, réservé à `admin` (`403 FORBIDDEN` sinon) ; toute autre valeur de `scope` : `400 VALIDATION_ERROR`.

```json
{ "id": "uuid", "userId": "uuid", "username": "admin", "ipAddress": "192.168.1.20",
  "userAgent": "Mozilla/5.0 …", "authMethod": "password", "createdAt": "…",
  "lastSeenAt": "…", "expiresAt": "…", "current": true }
```

  `userAgent` est tronqué à 256 octets. `current` désigne la session de la requête.
- `DELETE /api/v1/auth/sessions/{sessionID}` (UUID minuscule) : `200 { "revoked": true }`. Chacun révoque ses sessions, un administrateur celles de tous ; la session d'un autre compte est `404 NOT_FOUND` pour un non-administrateur. Révoquer la session courante efface aussi le cookie. Audité (`session`, UUID, `revoke`).
- `POST /api/v1/auth/sessions/revoke-others` : `200 { "revoked": 2 }`, toutes les sessions du compte sauf la courante. Audité (`user`, UUID, `revoke_other_sessions`) quand au moins une session est révoquée.

### 3.3 Politique de sécurité

- `GET /api/v1/security/policy` (admin) : `200 { "idleTimeoutMinutes": 60, "allowedNetworks": [], "requireAdminSecondFactor": false, "currentAddress": "192.168.1.20" }`. `currentAddress` est l'adresse vue par le Manager pour la requête.
- `PUT /api/v1/security/policy` (admin) avec `{ "idleTimeoutMinutes": 60, "allowedNetworks": ["192.168.1.0/24"], "requireAdminSecondFactor": true }`, les deux premiers champs obligatoires, `requireAdminSecondFactor` facultatif (valeur actuelle conservée s'il est absent ; l'activer exige que l'administrateur qui modifie ait lui-même un second facteur, sinon `409 SECOND_FACTOR_NOT_ENROLLED`) : `idleTimeoutMinutes` de 5 à 720 ; `allowedNetworks` 32 préfixes CIDR au plus, stockés masqués ; liste vide = toutes les adresses. Succès `200` (même objet que `GET`), avec audit (`security_policy`, `local`, `update`). Une politique qui exclurait `currentAddress` : `409 POLICY_EXCLUDES_CURRENT_ADDRESS`.
- **Réseaux autorisés** — avec une liste non vide, toute requête d'une adresse hors liste reçoit `403 NETWORK_NOT_ALLOWED`, interface comprise, sauf `GET /health`. L'adresse est celle de la connexion TCP (`RemoteAddr`) : derrière un proxy inverse, c'est celle du proxy ; aucun en-tête `X-Forwarded-For` n'est pris en compte.

### 3.4 Journal d'audit

- `GET /api/v1/audit` (admin) : `200 { "entries": [Entrée] }`, les plus récentes d'abord. Filtres facultatifs : `actor`, `targetType`, `action`, `outcome` (égalité exacte, 255 octets au plus), `from` et `to` (RFC 3339, intervalle `[from, to)`, `from` avant `to`), `limit` (1 à 1 000, défaut 200). Sinon `400 VALIDATION_ERROR`.

```json
{ "id": "uuid", "actor": "alice", "targetType": "route", "targetId": "GET /api/v1/users",
  "action": "access", "outcome": "denied", "requestId": "host/abc-000012", "createdAt": "…" }
```

- `GET /api/v1/audit/export?format=csv|json` (admin), mêmes filtres, 10 000 lignes au plus (défaut et maximum) : fichier joint `audit-<horodatage>.csv` ou `.json`. Le CSV a l'en-tête `createdAt,actor,targetType,targetId,action,outcome,requestId` ; une valeur commençant par `=`, `+`, `-`, `@`, une tabulation ou un retour chariot est préfixée d'une apostrophe (pas de formule dans un tableur).
- Résultats tracés : `success`, `denied` (rôle insuffisant : cible `route`, `METHODE chemin`, action `access` ; dernier administrateur protégé : cible `user`), `requested` et `failure` (alimentation, § 4.5).

### 3.5 Second facteur

TOTP (RFC 6238 : HMAC-SHA-1, 6 chiffres, 30 s, ±1 période, un code n'est accepté qu'une fois) et 10 codes de secours à usage unique. WebAuthn est prévu en phase 8.

- **Deuxième étape de connexion** — `POST /api/v1/auth/login/second-factor` avec `{ "method": "totp" | "recovery", "code": "123456" }` (1 à 64 octets ; espaces ignorés pour le TOTP, casse, espaces et tirets ignorés pour un code de secours) et le cookie de défi `homelab_login` (`HttpOnly`, `SameSite=Strict`, `Path=/api/v1/auth`, 5 minutes, 5 essais, gardé en mémoire : un redémarrage du Manager impose de ressaisir le mot de passe). Succès `200 { "user": … }`, cookie de session et méthode de session `password+totp` ou `password+recovery`. Code faux : `401 INVALID_SECOND_FACTOR`, compté dans le verrouillage du compte (§ 3.2). Défi absent, expiré, épuisé ou compte devenu indisponible : `401 LOGIN_CHALLENGE_EXPIRED`. TOTP sans clé maître : `503 SECOND_FACTOR_UNAVAILABLE` (les codes de secours restent acceptés). Même limitation par adresse et même contrôle d'origine que la connexion.
- `GET /api/v1/auth/second-factor` : `200 { "enabled": false, "pending": false, "recoveryCodesRemaining": 0, "available": true }`. `available` est faux quand la clé maître manque (§ 2.5 de la spécification).
- `POST /api/v1/auth/second-factor/totp` avec `{ "password": "..." }` : `200 { "secret": "BASE32…", "uri": "otpauth://totp/Homelab%20Manager:alice?…", "qr": { "size": 29, "rows": ["1111111010…", …] } }`. `rows` décrit les modules du QR code (`1` = foncé), à dessiner par l'interface. Remplace un enrôlement en attente. Mot de passe faux : `401 INVALID_CREDENTIALS` ; déjà activé : `409 SECOND_FACTOR_ALREADY_ENABLED` ; sans clé maître : `503 SECOND_FACTOR_UNAVAILABLE`.
- `POST /api/v1/auth/second-factor/totp/confirm` avec `{ "code": "123456" }` : `200 { "recoveryCodes": ["k7pq-2mzx-b9ht", …] }` (10 codes, affichés cette seule fois). Code faux : `400 INVALID_SECOND_FACTOR` ; aucun enrôlement en attente : `409 NO_PENDING_ENROLLMENT`. Audité (`user`, UUID, `second_factor_enable`).
- `POST /api/v1/auth/second-factor/recovery-codes` avec `{ "password" }` : `200 { "recoveryCodes": […] }`, les anciens codes ne valent plus. Sans second facteur : `409 SECOND_FACTOR_NOT_ENABLED`. Audité (`recovery_codes_regenerate`).
- `POST /api/v1/auth/second-factor/disable` avec `{ "password" }` : `200 { "disabled": true }`, secret et codes supprimés. Audité (`second_factor_disable`).

## 4. Santé, synthèse, hôte et paramètres

### 4.1 Santé

`GET /health` : `200 { "status": "ok", "service": "homelab-manager", "timestamp": "...", "uptime": "12h0m0s" }` tant que le processus répond.

`GET /api/v1/health` :

```json
{
  "status": "degraded",
  "service": "homelab-manager",
  "timestamp": "2026-11-21T20:58:00Z",
  "uptime": "12h0m0s",
  "database": "ok",
  "collectors": {
    "system": { "status": "ok", "lastSuccessAt": "2026-11-21T20:58:00Z" },
    "docker": { "status": "unavailable", "lastSuccessAt": null }
  }
}
```

- Toujours HTTP 200. `status` vaut `ok` seulement si `database`, `collectors.system.status` et `collectors.docker.status` valent tous `ok`, sinon `degraded`.
- `uptime` : durée Go arrondie à la seconde depuis le démarrage du processus.
- Aucun secret, chemin de fichier ou de socket.

**États d'un collecteur** (`status`), communs à toute l'API :

| Valeur | Signification |
|---|---|
| `starting` | Aucun cycle terminé depuis le démarrage |
| `ok` | Dernier cycle réussi, il y a moins de 3 intervalles |
| `stale` | Dernier succès plus ancien que 3 intervalles (système : 3 × `system_collect_interval_seconds` ; Docker : 3 × `docker_discovery_interval_seconds`) |
| `unavailable` | Dernier cycle en échec, ou base indisponible |

`lastSuccessAt` est l'heure du dernier succès, conservée lorsqu'un cycle échoue ; `null` si aucun succès. `database` prend les valeurs `starting`, `ok`, `unavailable` : il reflète le résultat de la dernière écriture des collecteurs ou de la purge.

### 4.2 Synthèse

`GET /api/v1/dashboard` :

```json
{
  "host": { "...": "DTO hôte du § 4.3" },
  "containers": { "total": 4, "running": 3, "stopped": 1, "other": 0 },
  "metrics": [
    { "name": "cpu.percent", "value": 21.4, "unit": "percent", "labels": { "core": "all" }, "recordedAt": "2026-11-21T20:58:00Z" }
  ],
  "collectors": {
    "system": { "status": "ok", "lastSuccessAt": "2026-11-21T20:58:00Z" },
    "docker": { "status": "ok", "lastSuccessAt": "2026-11-21T20:58:00Z" }
  }
}
```

- `containers` porte sur les conteneurs non supprimés : `running` = état `running` ; `stopped` = `exited` ou `created` ; `other` = tous les autres états (`paused`, `restarting`, `dead`, …).
- `metrics` contient **tous** les échantillons du dernier cycle système, s'il date de moins de 5 minutes, sinon `[]`. C'est le même contenu que `GET /api/v1/host/metrics/current`.

### 4.3 Hôte

```json
{
  "hostname": "nas",
  "os": { "distribution": "debian", "version": "13" },
  "kernelVersion": "6.12.0",
  "bootTime": "2026-11-20T08:00:00Z",
  "uptimeSeconds": 129600,
  "collector": { "status": "ok", "lastSuccessAt": "2026-11-21T20:58:00Z" }
}
```

`distribution`, `version`, `kernelVersion` valent `""` si la source locale ne les fournit pas ; `bootTime` et `uptimeSeconds` valent `null`. `uptimeSeconds` est recalculé à chaque requête depuis `bootTime`. `distribution` est l'identifiant de plateforme lu localement (ex. `debian`, `fedora`).

### 4.4 Paramètres effectifs

`GET /api/v1/settings` :

```json
{
  "systemCollectIntervalSeconds": 10,
  "dockerCollectIntervalSeconds": 15,
  "dockerDiscoveryIntervalSeconds": 30,
  "metricsRetentionDays": 7,
  "metricsRetentionDefaultDays": 7,
  "metricsRetentionSource": "config",
  "collectors": {
    "system": { "status": "ok", "lastSuccessAt": "..." },
    "docker": { "status": "ok", "lastSuccessAt": "..." }
  }
}
```

- Les intervalles viennent du fichier de configuration ou des variables d'environnement (README, « Configuration ») et ne changent qu'au redémarrage du service.
- `metricsRetentionDays` est la rétention effective. `metricsRetentionDefaultDays` est celle de la configuration (`metrics_retention_days`, 7 par défaut). `metricsRetentionSource` vaut `config` tant qu'aucune valeur n'a été choisie dans l'interface, puis `user`.

**Modifier la rétention** — `PUT /api/v1/settings/retention` avec `{ "metricsRetentionDays": 14 }` :

- Entier de 1 à 365, obligatoire ; sinon `400 VALIDATION_ERROR`.
- La valeur est enregistrée en base (table `app_settings`), prend effet immédiatement sans redémarrage, survit aux redémarrages et l'emporte ensuite sur la configuration. Une purge est lancée aussitôt : réduire la rétention supprime définitivement les données plus anciennes.
- Chaque modification ajoute une entrée au journal d'audit (`audit_logs` : acteur, cible `settings/metrics_retention_days`, action `update`, `requestId`), dans la même transaction.
- Réponse `200` : le DTO de paramètres ci-dessus. Échec d'écriture : `500 SETTINGS_UPDATE_FAILED`.

### 4.5 Alimentation de la machine

Réservé au rôle `admin` ; voir SPEC_ADMINISTRATION.md § 8 bis.

`GET /api/v1/machine/power` : `200 { "enabled": false, "actions": ["reboot", "poweroff"] }`. `enabled` reflète le réglage de service `power_actions_enabled` (défaut `false`) ; la règle polkit, elle, n'est connue qu'au moment de la demande.

`POST /api/v1/machine/power/{action}` avec `{ "confirmHostname": "homelab" }` (1 à 253 octets, espaces de bord ignorés, comparé exactement au nom d'hôte de la machine) :

- Succès `202 { "requested": true, "action": "reboot" }` : `systemd-logind` a accepté la demande ; le Manager s'arrête avec les autres services peu après. La réponse peut ne pas parvenir si l'arrêt est très rapide.
- `400 HOSTNAME_MISMATCH` : confirmation différente du nom d'hôte. `409 POWER_ACTIONS_DISABLED` : réglage désactivé. `409 ACTION_IN_PROGRESS` : une demande a déjà été acceptée. `503 POWER_ACTION_REFUSED` : polkit ou logind refuse (règle absente, verrou d'inhibition, arrêt déjà en cours). `500 POWER_ACTION_FAILED` : bus système ou logind injoignable.
- Une ligne d'audit (`host`, nom d'hôte, action, `requested`) est écrite avant la demande ; un refus ajoute `failure`.

### 4.6 Journal système

Lecture seule du journal systemd par `journalctl` (SPEC_ADMINISTRATION.md § 4), ouverte à tous les rôles. Le Manager doit appartenir au groupe `systemd-journal` ; sinon il ne voit que ses propres entrées (`fullAccess: false`). Sans `journalctl` dans son `PATH`, toutes ces routes répondent `503 JOURNAL_UNAVAILABLE` et le reste du Manager fonctionne.

**Filtres** (paramètres de requête, communs à `entries` et `export`) :

| Paramètre | Valeurs | Défaut |
|---|---|---|
| `source` | `all`, `kernel` (`_TRANSPORT=kernel`), `auth` (facilités `auth` et `authpriv`), `docker` (unité `docker.service`, exclusif de `unit`) | `all` |
| `priority` | Priorité la moins grave affichée : `emerg`, `alert`, `crit`, `err`, `warning`, `notice`, `info`, `debug` | `info` |
| `unit` | Nom d'unité systemd, motif `^[A-Za-z0-9@._:\-]+\.(service\|socket\|timer\|mount\|automount\|swap\|path\|scope\|slice\|target\|device)$`, 255 octets au plus | — |
| `boot` | Identifiant de démarrage (32 caractères hexadécimaux minuscules) présent dans `GET /journal/boots` | tous |
| `from`, `to` | RFC 3339, intervalle `[from, to)` | — |
| `q` | Texte cherché (200 octets au plus) : sous-chaîne insensible à la casse du message, de l'unité ou de l'identifiant, cherchée par le Manager | — |

La chaîne de requête est analysée strictement : un curseur doit être encodé (`;` → `%3B`), sinon `400 VALIDATION_ERROR`. Toute valeur hors de ces bornes, un démarrage inconnu ou `before` avec `after` → `400 VALIDATION_ERROR`.

`GET /api/v1/journal/entries` — en plus des filtres : `limit` (1 à 500, défaut 200) et au plus un curseur, `before` (entrées plus anciennes) ou `after` (entrées plus récentes, pour le suivi en direct). Réponse `200` :

```json
{
  "entries": [
    { "cursor": "s=…;i=…;b=…;m=…;t=…;x=…", "time": "2026-11-23T09:30:12.123456Z", "priority": 3,
      "unit": "ssh.service", "identifier": "sshd", "pid": "812", "hostname": "homelab",
      "bootId": "bbbb…", "message": "…", "truncated": false }
  ],
  "olderCursor": "s=…", "newerCursor": "s=…", "scanned": 201, "scanLimitReached": false, "skipped": 0
}
```

- `entries` va toujours de la plus récente à la plus ancienne. `priority` est `null` si l'entrée n'en a pas. `message` est tronqué à 8 Kio (`truncated: true`) ; l'entrée complète est donnée par `GET /journal/entry`.
- `olderCursor` : à passer en `before` pour la page suivante ; `null` quand il n'y a plus d'entrées. Absent (`null`) pour une page `after`.
- `newerCursor` : à passer en `after` pour le suivi ; c'est la dernière entrée examinée, même si la recherche ne l'a pas retenue, ou le `after` reçu si rien de nouveau. `null` pour une page `before`, ou si le journal est vide.
- Avec `q`, 5 000 entrées au plus sont examinées par requête ; `scanLimitReached: true` signale que la recherche s'est arrêtée à cette borne (ou à la taille maximale de sortie) : continuer avec `olderCursor`. `scanned` compte les entrées examinées, `skipped` celles illisibles ou de plus de 1 Mio.

`GET /api/v1/journal/entry?cursor=…` : `200 { "entry": Entrée, "fields": [{ "name": "_CMDLINE", "value": "[masqué]", "masked": true }, …] }`, champs triés par nom. Sont masqués `_CMDLINE` et tout champ dont le nom contient `PASSWORD`, `TOKEN` ou `SECRET` (casse ignorée). Une valeur binaire est rendue en texte UTF-8 pour `MESSAGE`, sinon `[données binaires, N octets]` ; un champ répété donne ses valeurs séparées par des sauts de ligne. Curseur invalide : `400` ; entrée disparue : `404 NOT_FOUND`.

`GET /api/v1/journal/boots` : `200 { "boots": [{ "index": 0, "bootId": "…", "firstEntry": "…", "lastEntry": "…" }, …] }`, du plus récent (`index` 0, démarrage en cours) au plus ancien, 100 au plus. Liste gardée une minute.

`GET /api/v1/journal/units` : `200 { "units": ["cron.service", …] }`, unités distinctes du champ `_SYSTEMD_UNIT`, triées, 1 000 au plus. Elle reste la source des filtres du journal : l'inventaire du § 4.7 ne contient que les services, timers et sockets chargés, alors que le journal cite aussi des unités d'autres types ou déchargées.

`GET /api/v1/journal/summary` : `200 { "start", "bucketMinutes": 15, "buckets": [{ "start", "counts": [emerg, alert, crit, err, warning, notice, info, debug] }, … 96 tranches], "total", "truncated", "diskUsageBytes", "fullAccess", "generatedAt" }`. Les 96 tranches couvrent les 24 h écoulées, la dernière contient l'heure courante ; une entrée sans priorité compte comme `info`. Au-delà de 200 000 entrées, seules les plus récentes sont comptées (`truncated: true`). `diskUsageBytes` (`journalctl --disk-usage`) est `null` si la lecture échoue. Résultat gardé une minute.

`GET /api/v1/journal/export?format=log|json` et les filtres : fichier joint `journal-<horodatage>.log` (format court `2026-11-23T09:30:12Z hôte identifiant[pid]: message`, lignes suivantes indentées) ou `.json` (tableau d'entrées comme ci-dessus), dans l'ordre chronologique. Seules les 50 000 entrées les plus récentes du filtre sont lues ; la recherche `q` s'applique à celles-ci. La réponse est envoyée en flux : une erreur après le début (délai de 10 s de `journalctl` dépassé, par exemple) coupe la réponse, et un export JSON coupé reste alors invalide.

Bornes communes : 10 s par appel à `journalctl`, 4 appels simultanés au plus, 64 Mio de sortie par appel. Échec de `journalctl` : `500 JOURNAL_READ_FAILED` (le détail va dans le journal du Manager).

### 4.7 Services système

Lecture des unités systemd par D-Bus (SPEC_ADMINISTRATION.md § 5.1), ouverte à tous les rôles. Pour la lecture, le Manager appelle seulement `ListUnits`, `GetUnit` et la lecture des propriétés (`org.freedesktop.DBus.Properties.GetAll` et `Get`) sur `org.freedesktop.systemd1`. Les seules méthodes qui modifient une unité sont les quatre actions de la liste blanche (§ 5.2, plus bas).

**Collecte** : au démarrage puis toutes les 30 s (25 s au plus par cycle), indépendamment des requêtes. Seuls les types `service`, `timer` et `socket` chargés en mémoire par systemd sont retenus, 2 000 au plus, les unités en échec d'abord puis par nom.

`GET /api/v1/system/units` :

```json
{
  "status": "ok",
  "collectedAt": "2026-11-23T10:00:00Z",
  "counts": { "total": 214, "active": 131, "failed": 1, "activeTimers": 12 },
  "units": [
    {
      "name": "ssh.service", "description": "OpenBSD Secure Shell server", "type": "service",
      "loadState": "loaded", "activeState": "active", "subState": "running", "unitFileState": "enabled",
      "stateChangedAt": "2026-11-23T08:00:00.123456Z", "mainPid": 812, "memoryBytes": 4194304,
      "cpuNanoseconds": 2000000000, "result": "success", "nextElapse": null, "lastTrigger": null, "triggers": []
    }
  ],
  "manageableUnits": ["nginx.service"]
}
```

- `status` : `pending` (aucun cycle terminé depuis le démarrage), `ok`, `error` (systemd a répondu par une erreur ou le cycle a dépassé son délai), `unavailable` (pas de bus système ou pas de systemd, par exemple hors Linux, ou systemd installé mais non démarré : son activation par le bus échoue). Après un échec, `units`, `counts` et `collectedAt` restent ceux du dernier cycle réussi ; `collectedAt` est `null` tant qu'aucun cycle n'a réussi.
- `counts` : `active` et `failed` comptent les `activeState` `active` et `failed` ; `activeTimers`, les timers `active`.
- Valeurs absentes, jamais `0` : `mainPid` (pas de processus principal), `memoryBytes` et `cpuNanoseconds` (comptabilité désactivée, `UINT64_MAX` pour systemd ; services seulement), `stateChangedAt`, `nextElapse` et `lastTrigger` (horodatage 0, jamais ; ces deux derniers pour les timers seulement), `unitFileState` et `result` (non fournis) valent `null`.
- `triggers` : unités démarrées par un timer ou un socket ; `[]` sinon.
- `manageableUnits` : liste blanche de la configuration du service (`systemd_manageable_units`), triée ; `[]` si elle est vide ou si les actions sont indisponibles. Elle peut citer une unité absente de l'inventaire (non chargée).
- Une unité disparue entre la liste et la lecture de ses propriétés garde les seuls champs de la liste (`description`, états).

`GET /api/v1/system/units/{unitName}` — unité du dernier inventaire, relue à la demande (10 s au plus) :

```json
{
  "unit": { "...": "DTO d'unité ci-dessus" },
  "definition": {
    "fragmentPath": "/lib/systemd/system/ssh.service",
    "execStart": ["/usr/sbin/sshd -D"],
    "requires": ["sysinit.target"], "after": ["network.target", "auditd.service"],
    "restart": "on-failure", "slice": "system.slice",
    "tasksMax": { "value": null, "unlimited": true },
    "limitNofile": { "value": 1024, "unlimited": false },
    "memoryMax": { "value": null, "unlimited": false }
  },
  "journal": { "available": true, "entries": [{ "...": "entrée du § 4.6" }] }
}
```

- `execStart` : chaque commande telle que systemd la conserve, arguments joints par des espaces ; un argument vide ou contenant une espace, une tabulation, un saut de ligne ou un guillemet est placé entre guillemets doubles (`\` et `"` échappés) pour que chaque argument reste distinct. Affichée sans être exécutée ni interprétée. `requires` et `after` : 100 noms au plus.
- Limites : `unlimited: true` pour `infinity` ; `value: null` et `unlimited: false` quand la limite est inconnue (propriété absente, par exemple pour un timer). `restart`, `slice` et `fragmentPath` sont `null` s'ils ne sont pas fournis.
- `journal.entries` : les 50 entrées les plus récentes de l'unité, toutes priorités, de la plus récente à la plus ancienne, au format du § 4.6. `available: false` et `entries: []` si le journal ne peut pas être lu (`journalctl` absent ou en échec) ; le détail reste servi.
- Erreurs : nom invalide `400 VALIDATION_ERROR` (un nom échappé par systemd, contenant `\`, est hors du motif : l'unité figure dans l'inventaire mais son détail est refusé) ; unité absente du dernier inventaire ou disparue `404 NOT_FOUND` ; aucun inventaire réussi ou systemd injoignable `503 SYSTEMD_UNAVAILABLE` ; autre échec de lecture `500 SYSTEMD_READ_FAILED`.

**Actions** (SPEC_ADMINISTRATION.md § 5.2, phase 5) : `start`, `stop`, `restart` ou `reload` d'une unité de `manageableUnits`, par D-Bus (`StartUnit`, `StopUnit`, `RestartUnit`, `ReloadUnit`, mode `replace`, sans authentification interactive), autorisé par la règle polkit que génère `homelab-manager polkit-rule`. « Exécuter » un timer est un `start` du service qu'il déclenche, demandé par l'interface sur ce service.

`POST /api/v1/system/units/{unitName}/actions` (`operator` ou `admin`) avec `{ "action": "restart" }` → `202 { "action": Action }` (`state: running`). L'action se poursuit en arrière-plan : le Manager suit le job puis l'unité jusqu'à un état stable (ni `activating`, ni `deactivating`, ni `reloading`), 90 s au plus. Action :

```json
{
  "unit": "nginx.service", "action": "restart", "state": "succeeded", "failure": null,
  "requestedBy": "olivia", "requestedAt": "2026-11-24T10:00:00Z", "finishedAt": "2026-11-24T10:00:03Z",
  "final": { "activeState": "active", "subState": "running" }
}
```

- `state` : `running`, `succeeded` (unité qui n'est pas `failed` à l'état stable), `failed`. `failure` (`null` sauf en échec) : `failed` (unité en échec), `not_allowed` (refus de polkit : règle absente ou sans cette unité), `not_found` (unité inconnue de systemd), `rejected` (job refusé par systemd : verbe non applicable, unité masquée, démarrage manuel interdit…), `timeout`, `unavailable` (bus ou systemd injoignable), `interrupted` (Manager arrêté pendant le suivi). `final` : état lu à la fin, `null` en cours ou s'il n'a pas pu être lu ; une unité déchargée après un arrêt vaut `inactive` / `dead`.
- Audit (`system_unit` / nom de l'unité / verbe) : `requested` avant l'appel à systemd (sans écriture possible, rien n'est demandé), puis `success` ou `failure` à la fin.
- Erreurs : corps invalide, verbe inconnu ou champ inconnu `400 VALIDATION_ERROR` ; nom invalide `400 VALIDATION_ERROR` ; unité hors liste blanche `409 UNIT_NOT_MANAGEABLE` ; action déjà en cours sur l'unité `409 ACTION_IN_PROGRESS` ; plus de 30 demandes dans la dernière minute pour l'utilisateur `429 UNIT_ACTION_RATE_LIMITED` ; rôle `viewer` `403 FORBIDDEN` (audité) ; sans base de données, ou Manager en cours d'arrêt, `503 UNIT_ACTIONS_UNAVAILABLE` ; audit impossible à écrire `500 UNIT_ACTION_FAILED`.

`GET /api/v1/system/units/{unitName}/actions/last` (tous les rôles) → `200 { "action": Action }` : dernière action demandée sur l'unité depuis le démarrage du Manager (gardée en mémoire ; l'historique est dans le journal d'audit). Aucune : `404 NOT_FOUND`. L'interface l'interroge chaque seconde tant que `state` vaut `running`.

### 4.8 Alertes

Moteur d'alertes intégré (SPEC_ADMINISTRATION.md § 6), évalué toutes les 15 s sur les données déjà collectées, sans requête d'historique ; notifications par canaux (phase 4B), seule communication sortante du Manager. Sans base de données, toutes ces routes répondent `503 ALERTS_UNAVAILABLE`.

**Règle** :

```json
{
  "id": "6f1c2a10-7a4e-4c4b-9a51-000000000001", "name": "Disque presque plein",
  "type": "host_metric", "severity": "warning", "enabled": true, "default": true,
  "params": { "metric": "filesystem.percent", "labels": { "mountpoint": "/" }, "operator": ">", "threshold": 85, "forSeconds": 300 },
  "createdAt": "2026-11-24T00:00:00Z", "updatedAt": "2026-11-24T00:00:00Z"
}
```

| `type` | Paramètres (`params`) | Condition vraie |
|---|---|---|
| `host_metric` | `metric`, `labels` (facultatif), `operator`, `threshold`, `forSeconds` | Une série du dernier cycle hôte compare vrai au seuil ; chaque série (labels) est une cible |
| `container_metric` | `containerId` (facultatif : tous), `metric`, `labels`, `operator`, `threshold`, `forSeconds` | Idem sur le dernier cycle de chaque conteneur |
| `container_state` | `containerId` (facultatif), `states` (non vide, valeurs distinctes), `forSeconds` | L'état Docker du conteneur est l'un des `states` |
| `unit_failed` | `unitPattern` (nom ou motif shell `*`, `?`, `[…]`) | Une unité de l'inventaire qui correspond est en `failed` ; immédiat |
| `collector` | `collector` (`system`, `docker`, `services`), `forSeconds` | Le collecteur n'est ni `ok`, ni en démarrage (`starting`, `pending`) |
| `vulnerabilities` | `vulnSeverity` (`critical`, `high`, `medium`, `low` : sévérité minimale), `fixableOnly` (facultatif) | Au moins un constat présent (§ 4.9) de cette sévérité ou plus grave, corrigeable si `fixableOnly` ; immédiat. Gelée tant qu'aucun scan n'a réussi. Une alerte par règle (cible `vulnerabilities`), `value` = nombre de constats |

- `name` : 1 à 100 octets sans caractère de contrôle ; `severity` : `info`, `warning`, `critical` ; `operator` : `>`, `>=`, `<`, `<=` ; `threshold` : nombre fini ; `forSeconds` : 0 à 86 400 (durée pendant laquelle la condition doit rester vraie) ; `labels` : 5 au plus, clés parmi celles de la métrique, valeurs de 1 à 255 octets. Un paramètre qui n'appartient pas au type est refusé.
- Métriques : celles du catalogue renvoyé par `GET /api/v1/alert-rules` (`catalog.hostMetrics` et `catalog.containerMetrics`, chacune avec ses labels), sous-ensemble des § 5.4 et § 5.5 sans compteurs cumulés ni totaux, plus `memory.available_percent` (hôte), dérivée par le moteur : `memory.available / memory.total × 100`. `catalog.containerStates`, `catalog.collectors` et `catalog.vulnerabilitySeverities` listent les autres valeurs acceptées.
- Six règles par défaut (`default: true`) : disque `filesystem.percent > 85` 5 min, mémoire `memory.available_percent < 10` 5 min (`warning`) ; température `temperature.celsius > 85` 2 min (`critical`, seuil fixe : les seuils des capteurs ne sont pas collectés) ; collecte système et collecte Docker indisponibles 2 min (`warning`) ; unité en échec, motif `*` (`critical`). Modifiables et désactivables, non supprimables.

**Alerte** :

```json
{
  "id": "uuid", "ruleId": "uuid", "ruleName": "Disque presque plein", "ruleType": "host_metric",
  "severity": "warning", "state": "firing",
  "target": { "kind": "host", "metric": "filesystem.percent", "labels": { "mountpoint": "/", "device": "sda1" },
              "containerId": null, "containerName": null, "unit": null, "collector": null },
  "value": 91.5, "threshold": 85, "observed": null,
  "firedAt": "2026-11-24T10:05:00Z", "acknowledgedAt": null, "acknowledgedBy": null,
  "resolvedAt": null, "resolution": null,
  "targetKey": "host:filesystem.percent{device=sda1,mountpoint=/}", "silenced": false, "lastNotifiedAt": "2026-11-24T10:05:04Z"
}
```

- `target.kind` : `host`, `container`, `unit`, `collector` ou `vulnerabilities` (paquets de la machine). `value` et `threshold` : règles de métrique (valeur au déclenchement) ; `observed` : état qui a déclenché les autres règles (`exited`, `failed (exit-code)`, `unavailable`). `ruleName`, `ruleType` et `severity` sont copiés au déclenchement.
- Cycle : `firing` → `acknowledged` (facultatif) → `resolved`. Une seule alerte ouverte par règle et par cible ; une condition de nouveau vraie après la résolution ouvre une nouvelle alerte. `resolution` : `condition_cleared` (condition fausse), `no_data` (série absente d'un cycle à jour depuis 5 min), `target_gone` (conteneur ou unité sorti de l'inventaire), `rule_disabled`, `rule_deleted`, `rule_changed` (type, sévérité ou paramètres modifiés ; un simple renommage garde les alertes).
- `targetKey` : clé stable de la cible, reprise par un silence de portée `target`. `silenced` : un silence en vigueur couvre l'alerte ouverte (toujours `false` pour une alerte résolue) ; l'alerte reste listée, seules ses notifications sont suspendues. `lastNotifiedAt` : dernier message envoyé à son sujet, tous canaux confondus, `null` sinon.
- Une source illisible ou périmée (aucun cycle hôte de moins de 5 min, collecte Docker hors `ok`, inventaire systemd hors `ok`) ne déclenche ni ne résout les règles qui en dépendent. Le début d'une condition qui n'a pas encore tenu sa durée est gardé en mémoire : après un redémarrage du Manager, la durée recommence ; les alertes ouvertes, elles, sont conservées.

`GET /api/v1/alerts[?state=open|resolved|all&from=…&to=…&limit=…]` → `{ "alerts": [...] }`, de la plus récente à la plus ancienne (`firedAt`). `state` par défaut `all` ; `from` et `to` RFC 3339 facultatifs, intervalle `[from, to)` sur `firedAt` ; `limit` 1 à 1 000, défaut 200. Tout autre paramètre, ou un paramètre répété → `400 VALIDATION_ERROR`.

`GET /api/v1/alerts/summary` → `{ "open": { "info": 0, "warning": 1, "critical": 0 }, "resolvedLast24h": 3, "meanResolutionSeconds24h": 840 }` ; `meanResolutionSeconds24h` est `null` sans alerte résolue sur 24 h.

`POST /api/v1/alerts/{alertID}/acknowledge` (sans corps, `operator` ou `admin`) → `200 { "alert": Alerte }`. L'alerte reste ouverte jusqu'à sa résolution. Déjà acquittée : renvoyée inchangée. Résolue : `409 ALERT_ALREADY_RESOLVED`. Audité (`alert` / UUID / `acknowledge`).

`GET /api/v1/alert-rules` → `{ "rules": [...], "catalog": { "hostMetrics": { "filesystem.percent": ["mountpoint", "device"], … }, "containerMetrics": { … }, "containerStates": [...], "collectors": [...] } }`, règles par défaut d'abord puis par nom.

`POST /api/v1/alert-rules` avec `{ "name", "type", "severity", "enabled", "params" }` (`enabled` obligatoire, champs inconnus refusés) → `201 { "rule": Règle }` ; `PUT /api/v1/alert-rules/{ruleID}` avec le même corps → `200 { "rule": Règle }` (une règle par défaut le reste) ; `DELETE /api/v1/alert-rules/{ruleID}` → `200 { "deleted": true }`, ses alertes ouvertes sont résolues (`rule_deleted`). Règle invalide : `400 VALIDATION_ERROR` avec le motif ; règle par défaut supprimée : `409 DEFAULT_RULE` ; au-delà de 200 règles : `409 TOO_MANY_RULES`. Chaque modification est auditée (`alert_rule` / UUID / `create`, `update`, `delete`) dans la même transaction.

**Silence** :

```json
{
  "id": "uuid", "scope": "target", "ruleId": "6f1c2a10-7a4e-4c4b-9a51-000000000001", "ruleName": "Disque presque plein",
  "targetKey": "host:filesystem.percent{device=sda1,mountpoint=/}", "reason": "Migration du disque",
  "createdBy": "otto", "startsAt": "2026-11-24T10:00:00Z", "endsAt": "2026-11-24T12:00:00Z",
  "liftedAt": null, "liftedBy": null, "active": true
}
```

- `scope` : `rule` (toutes les alertes d'une règle, `ruleId` seul), `target` (l'alerte d'une règle sur une cible, `ruleId` et `targetKey`) ou `all` (toutes les alertes, ni `ruleId` ni `targetKey`). Un silence suspend les notifications des alertes qu'il couvre, y compris les résolutions et les renotifications ; il ne change ni leur état ni leur affichage.
- `ruleName` : nom actuel de la règle, `null` pour un silence global ou une règle supprimée. `active` : en vigueur à la lecture.

`GET /api/v1/alert-silences` → `{ "silences": [...] }` : silences en vigueur puis terminés depuis moins de 7 jours, du plus récent au plus ancien, 200 au plus.

`POST /api/v1/alert-silences` avec `{ "scope", "ruleId", "targetKey", "durationMinutes", "reason" }` (`operator` ou `admin`, champs inconnus refusés) → `201 { "silence": Silence }`. Le silence commence aussitôt ; `durationMinutes` de 15 à 10 080 (7 jours) ; `reason` de 1 à 200 caractères sans caractère de contrôle ; `targetKey` de 1 à 512 octets ; `ruleId` en UUID minuscule d'une règle existante (sinon `404 NOT_FOUND`). Au-delà de 100 silences en vigueur : `409 TOO_MANY_SILENCES`. Audité (`alert_silence` / UUID / `create`).

`DELETE /api/v1/alert-silences/{silenceID}` (`operator` ou `admin`) → `200 { "silence": Silence }`, levé à l'instant (`liftedAt`, `liftedBy`). Un silence déjà terminé est renvoyé inchangé, sans audit. Audité (`alert_silence` / UUID / `lift`).

**Canal de notification** (réservé à `admin`) :

```json
{
  "id": "uuid", "name": "Téléphone", "type": "ntfy", "enabled": true, "minSeverity": "warning", "renotifyMinutes": 30,
  "settings": { "serverUrl": "https://ntfy.example.org", "topic": "homelab" },
  "secrets": { "accessToken": { "defined": true, "fingerprint": "3f9a0c1b7e42" } },
  "lastSentAt": "2026-11-24T10:05:04Z", "lastErrorAt": null, "lastError": null, "lastErrorStatus": null,
  "createdAt": "2026-11-24T09:00:00Z", "updatedAt": "2026-11-24T09:00:00Z"
}
```

| `type` | `settings` | `secrets` |
|---|---|---|
| `telegram` | `chatId` (nombre, éventuellement négatif, ou `@canal`) | `botToken` (obligatoire) |
| `discord` | — | `webhookUrl` (obligatoire, `https://discord.com/api/webhooks/…` ou `discordapp.com`) |
| `gotify` | `serverUrl` | `appToken` (obligatoire) |
| `ntfy` | `serverUrl`, `topic` (1 à 64 lettres, chiffres, `-`, `_`) | `accessToken` (facultatif) |
| `smtp` | `host`, `port` (1 à 65 535), `security` (`starttls` ou `tls`), `username` (facultatif), `from`, `to` (1 à 10 adresses distinctes) | `password` (avec `username` seulement) |
| `webhook` | `url` | `signingSecret` (facultatif, 16 octets au moins) |

- `name` : 1 à 100 octets sans caractère de contrôle ; `minSeverity` : sévérité minimale transmise ; `renotifyMinutes` : 0 (jamais) ou 5 à 1 440, intervalle de renotification d'une alerte `firing` non acquittée et hors silence. Un réglage qui n'appartient pas au type est refusé. Adresses (`serverUrl`, `url`) : `http` ou `https`, sans identifiants, requête ni fragment, 2 048 octets au plus ; les adresses privées sont permises. Adresses électroniques sans nom affiché. Secrets : 1 à 1 024 octets sans caractère de contrôle. 20 canaux au plus (`409 TOO_MANY_CHANNELS`).
- Les secrets ne sont jamais renvoyés : `secrets` liste chaque secret du type avec `defined` et `fingerprint` (12 premiers caractères hexadécimaux du SHA-256), `null` si le secret n'est pas défini ou si la clé maître manque. Sans clé maître (`secretsAvailable: false`), créer ou modifier un secret et tester un canal qui en a répondent `503 SECRETS_UNAVAILABLE` ; un canal sans secret reste utilisable.
- `lastSentAt` : dernier envoi réussi ; `lastErrorAt`, `lastError` et `lastErrorStatus` : dernier échec, conservé après un succès. `lastError` : `timeout`, `connection_failed`, `tls_failed`, `redirect_refused`, `http_status` (statut HTTP dans `lastErrorStatus`), `smtp_rejected` (code SMTP dans `lastErrorStatus`), `secrets_unavailable` ou `failed`. Aucune adresse, aucun jeton ni réponse du serveur n'est conservé.

`GET /api/v1/notification-channels` → `{ "channels": [...], "secretsAvailable": true }`, par nom.

`POST /api/v1/notification-channels` avec `{ "name", "type", "enabled", "minSeverity", "renotifyMinutes", "settings", "secrets" }` (`enabled` et `renotifyMinutes` obligatoires, champs inconnus refusés) → `201 { "channel": Canal }`. `PUT /api/v1/notification-channels/{channelID}` avec le même corps → `200 { "channel": Canal }` ; le type ne change pas (`400`). Dans `secrets`, une clé absente garde la valeur enregistrée, `null` supprime le secret, une chaîne le remplace. `DELETE /api/v1/notification-channels/{channelID}` → `200 { "deleted": true }`. Audités (`notification_channel` / UUID / `create`, `update`, `delete`) dans la même transaction.

`POST /api/v1/notification-channels/{channelID}/test` (sans corps) envoie aussitôt un message de test, même à un canal désactivé, et enregistre son résultat. Succès : `200 { "channel": Canal }`. Échec : `502 NOTIFICATION_FAILED` avec le code et le statut seulement (le canal relu porte `lastError`). Audité (`notification_channel` / UUID / `test`, `outcome` `success` ou `failure`).

**Envois** :

- Le moteur transmet chaque déclenchement et chaque résolution (sauf celles dues à une modification de règle) à une file bornée (1 000 transitions, au-delà elles sont comptées et journalisées). Toutes les 5 s, chaque transition est routée vers les canaux activés dont la sévérité minimale est atteinte, hors silence. Chaque canal envoie au plus un message par minute : les transitions arrivées entre-temps sont regroupées (200 au plus par canal, les plus anciennes au-delà sont comptées « … et N autres notifications »).
- Renotification : une fois par minute, une alerte `firing` (non acquittée), hors silence, dont le dernier message sur ce canal date d'au moins `renotifyMinutes` (à défaut, `lastNotifiedAt` ou `firedAt`) est renvoyée comme « toujours active ». Une alerte déclenchée pendant un silence est donc notifiée à la fin du silence si elle est toujours active et si le canal renotifie (`renotifyMinutes` > 0).
- Chaque envoi : 10 s au plus, TLS 1.2 au moins avec certificat vérifié, aucune redirection suivie, réponse lue sur 64 Kio au plus puis ignorée. Un échec est noté sur le canal, sans nouvel essai ; la supervision n'en dépend pas.
- Messages en français : titre `<machine> · <sévérité> · <règle>` (un seul élément) ou `<machine> · 2 déclenchées, 1 résolue`, puis un bloc par notification (événement, sévérité, règle ; cible, valeur et seuil ou état observé, durée). Telegram : `POST https://api.telegram.org/bot<jeton>/sendMessage` (4 096 caractères au plus). Discord : `POST` sur l'URL du webhook, mentions désactivées (2 000 caractères). Gotify : `POST <serverUrl>/message`, en-tête `X-Gotify-Key`, priorité 2, 5 ou 8. ntfy : publication JSON sur `<serverUrl>` (`topic`, `title`, `message`, `priority` 3, 4 ou 5), `Authorization: Bearer` avec un jeton. SMTP : `STARTTLS` exigé ou TLS immédiat, `AUTH PLAIN` avec un utilisateur, texte en `quoted-printable`, sujet encodé.
- Webhook : `POST` JSON, `Content-Type: application/json`, `User-Agent: Homelab-Manager` :

```json
{
  "version": 1, "host": "nas", "sentAt": "2026-11-24T10:05:04Z", "test": false, "omitted": 0,
  "notifications": [
    { "event": "firing", "alert": {
        "id": "uuid", "ruleId": "uuid", "ruleName": "Disque presque plein", "ruleType": "host_metric",
        "severity": "warning", "state": "firing", "targetKey": "host:filesystem.percent{device=sda1,mountpoint=/}",
        "target": { "kind": "host", "metric": "filesystem.percent", "labels": { "device": "sda1", "mountpoint": "/" } },
        "value": 91.5, "threshold": 85, "observed": null, "firedAt": "2026-11-24T10:05:00Z",
        "acknowledgedAt": null, "resolvedAt": null, "resolution": null } }
  ]
}
```

`event` : `firing`, `resolved` ou `reminder` ; `target` omet les champs vides ; `notifications` est vide pour un test (`test: true`) ; `omitted` compte les notifications retirées du message. Avec `signingSecret` : en-têtes `X-Homelab-Timestamp` (secondes Unix) et `X-Homelab-Signature: sha256=<hex>`, HMAC-SHA256 du secret sur `<timestamp>.<corps brut>`. Le récepteur recalcule la signature sur le corps reçu, la compare en temps constant et refuse un horodatage trop ancien (5 min par exemple).

### 4.9 Vulnérabilités des paquets

Vulnérabilités connues des paquets système de la machine du Manager (SPEC_ADMINISTRATION.md § 8 ter), trouvées par Grype dans l'unité dédiée `homelab-vulnscan.service`. Une vulnérabilité listée est **identifiée**, pas mesurée comme exploitable. Sans base de données, ces routes répondent `503 VULNERABILITIES_UNAVAILABLE`.

**Scan** :

```json
{
  "id": "uuid", "trigger": "schedule", "requestedBy": null, "state": "succeeded", "failure": null,
  "startedAt": "2026-11-24T10:00:00Z", "finishedAt": "2026-11-24T10:01:12Z",
  "grypeVersion": "0.121.3", "dbBuiltAt": "2026-11-24T04:12:00Z",
  "distro": { "id": "debian", "version": "13", "name": "Debian GNU/Linux 13 (trixie)" },
  "packageCount": 412,
  "totals": { "critical": 1, "high": 4, "medium": 12, "low": 30, "negligible": 55, "unknown": 2 },
  "newCount": 3, "resolvedCount": 1
}
```

- `trigger` : `startup` (premier scan, 2 min après le démarrage du Manager s'il n'existe aucun scan réussi), `schedule` (périodique), `retry` (nouvel essai unique 15 min après un échec), `manual`. `requestedBy` : compte d'un scan manuel.
- `state` : `running`, `succeeded`, `failed`. `failure` : `unsupported_distro` (distribution non prise en charge), `inventory_failed` (base de paquets illisible), `vulnscan_not_installed` (unité ou systemd absents), `vulnscan_not_allowed` (règle polkit absente), `timeout` (15 min dépassées), `scan_failed` (Grype en échec, dont base injoignable ou de plus de 5 jours : le journal de l'unité donne le détail), `stale_result` (aucun résultat récent), `invalid_result`, `interrupted` (Manager arrêté pendant le scan).
- `totals` : constats présents après un scan réussi, par sévérité ; `newCount` et `resolvedCount` : apparus et disparus par rapport au scan réussi précédent. `distro`, `grypeVersion` et `dbBuiltAt` sont `null` pour un scan qui n'a pas abouti.

**Constat** (une vulnérabilité d'une version de paquet) :

```json
{
  "id": "uuid", "vulnerabilityId": "CVE-2026-1000", "severity": "critical",
  "package": "openssl", "version": "3.5.1-1", "packageType": "deb",
  "fixState": "fixed", "fixedVersions": ["3.5.1-2"],
  "description": "…", "urls": ["https://security-tracker.debian.org/tracker/CVE-2026-1000"],
  "dataSource": "https://security-tracker.debian.org/tracker/CVE-2026-1000",
  "firstSeenAt": "2026-11-20T10:01:12Z", "lastSeenAt": "2026-11-24T10:01:12Z", "resolvedAt": null
}
```

- `severity` : `critical`, `high`, `medium`, `low`, `negligible`, `unknown`. `fixState` : `fixed` (une version corrige), `not-fixed`, `wont-fix`, `unknown`, d'après Grype. `description` (4 Kio au plus) et `dataSource` peuvent être `null` ; `urls` : 10 adresses `http`/`https` au plus.
- Un constat est présent tant que `resolvedAt` est `null` ; un scan réussi qui ne le trouve plus le résout. La même vulnérabilité retrouvée plus tard ouvre un nouveau constat. Constats résolus et scans conservés 180 jours (le dernier scan réussi toujours).

`GET /api/v1/vulnerabilities/summary` → `{ "totals": {…}, "total": 104, "fixable": 37, "latestScan": Scan | null, "lastSuccessfulScan": Scan | null, "running": false, "nextScanAt": "…" | null, "intervalHours": 24, "defaultIntervalHours": 24, "intervalSource": "config", "recent": [Constat × 5] }`. `intervalHours` : intervalle effectif des scans automatiques ; `defaultIntervalHours` : celui de la configuration (`vulnerability_scan_interval_hours`) ; `intervalSource` : `config`, ou `user` une fois l'intervalle choisi dans l'interface. `totals` et `fixable` comptent les constats présents ; `recent` : les 5 constats présents les plus récemment apparus ; `nextScanAt` est `null` pendant un scan. `latestScan` peut être un échec plus récent que `lastSuccessfulScan` : l'interface affiche les deux dates.

`GET /api/v1/vulnerabilities[?state=present|resolved&severity=critical,high&fixable=true&q=…&limit=…&offset=…]` → `{ "findings": [...], "total": 104, "limit": 100, "offset": 0 }`. `state` par défaut `present` (tri : sévérité puis apparition la plus récente) ; `resolved` trie par résolution la plus récente. `severity` : liste séparée par des virgules ; `q` : 100 caractères au plus, cherché sans casse dans l'identifiant et le nom du paquet ; `limit` 1 à 500 (défaut 100) ; `offset` 0 à 100 000. Tout autre paramètre, ou un paramètre répété → `400 VALIDATION_ERROR`.

`GET /api/v1/vulnerabilities/{findingID}` → `{ "finding": Constat }` ou `404 NOT_FOUND`.

`GET /api/v1/vulnerabilities/scans[?limit=…]` → `{ "scans": [...] }`, du plus récent au plus ancien, `limit` 1 à 100 (défaut 20).

`POST /api/v1/vulnerabilities/scans` (sans corps, `operator` ou `admin`) → `202 { "scan": Scan }` (`state: running`) ; le scan s'exécute en arrière-plan (une minute environ, 15 au plus). Un scan déjà en cours, automatique ou manuel : `409 SCAN_IN_PROGRESS` ; moins de 5 min après le précédent scan manuel : `429 SCAN_RATE_LIMITED`. Audité (`vulnerability_scan` / UUID / `start`).

`PUT /api/v1/settings/vulnerability-scans` avec `{ "intervalHours": 48 }` (`admin`, de 6 à 168, champs inconnus refusés) → `200 { "intervalHours": 48, "defaultIntervalHours": 24, "intervalSource": "user" }` ; appliqué aussitôt (le prochain scan est recalculé depuis la fin du dernier) et audité (`settings` / `vulnerability_scan_interval_hours` / `update`) dans la même transaction. Hors bornes : `400 VALIDATION_ERROR`.

## 5. Métriques

### 5.1 Échantillon

```json
{ "name": "cpu.percent", "value": 21.4, "unit": "percent", "labels": { "core": "all" }, "recordedAt": "2026-11-21T20:58:00Z" }
```

`labels` vaut `null` pour une série sans label. Une série est identifiée par `name` + `labels`. Tous les échantillons d'un même cycle portent le même `recordedAt` : un par cycle système, un par conteneur et par cycle de statistiques.

### 5.2 Valeurs courantes

`GET /api/v1/host/metrics/current` et `GET /api/v1/containers/{containerID}/metrics/current` :

```json
{ "samples": [ ... ], "status": "ok", "lastSuccessAt": "2026-11-21T20:58:00Z" }
```

- `samples` contient tous les échantillons du dernier cycle de l'hôte (ou du conteneur) enregistré depuis moins de 5 minutes, sinon `[]`. Au plus 5 000 échantillons.
- `status` et `lastSuccessAt` sont ceux du collecteur système (hôte) ou de la découverte Docker (conteneur), selon le § 4.1.
- Un conteneur arrêté n'a pas de statistiques : son dernier cycle ne contient que `restart.count`.

`GET /api/v1/host/temperatures` : `{ "samples": [...] }`, limité aux échantillons `temperature.celsius` du dernier cycle hôte.

### 5.3 Historique

`GET …/metrics/{metricName}?from=…&to=…[&limit=…][&step=…][&label=clé:valeur…]` → `{ "samples": [...], "nextCursor": null, "resolution": "raw" | "1m" | "5m" | "1h" }`.

- `from`, `to` : obligatoires, RFC 3339 avec n'importe quel décalage (normalisé en UTC). Intervalle semi-ouvert `[from, to)`. `from` doit précéder `to`, écart de 365 jours au plus.
- `limit` : entier de 1 à 2 000, 500 par défaut.
- `step` : `raw` (défaut), `1m`, `5m` ou `1h`. Autre valeur → `400 VALIDATION_ERROR`.
- `label` : répétable (4 au plus), format `clé:valeur`, clé `^[a-z0-9_]{1,32}$`, valeur de 1 à 128 caractères, clé non répétée. Ne garde que les séries portant tous ces labels (ex. `label=core:all`). Sans `label`, toutes les séries du nom sont renvoyées.
- `cursor` : toute valeur non vide → `400 VALIDATION_ERROR` (pagination non implémentée, § 9).
- Tout autre paramètre, ou paramètre répété autre que `label` → `400 VALIDATION_ERROR`.

**Brut (`step=raw`)** — si l'intervalle contient plus de `limit` échantillons, seuls les `limit` plus récents sont renvoyés, dans l'ordre chronologique. Une série de l'hôte produit 360 points par heure à l'intervalle par défaut.

**Agrégé (`1m`, `5m`, `1h`)** — un échantillon par série (`name` + `labels`) et par intervalle non vide, intervalles alignés sur l'époque Unix en UTC :

```json
{ "name": "cpu.percent", "value": 13.9, "min": 6.6, "max": 39.9, "count": 21, "unit": "percent", "labels": { "core": "all" }, "recordedAt": "2026-11-22T20:05:00Z" }
```

`value` est la moyenne, `recordedAt` le début de l'intervalle ; tri chronologique, puis par labels. Si le résultat dépasse `limit`, la requête échoue en `400 TOO_MANY_POINTS` au lieu de tronquer. `min`, `max` et `count` n'existent que sur les échantillons agrégés.

**Pas recommandés au client** : ≤ 1 h → `raw` ; ≤ 6 h → `1m` ; ≤ 24 h → `5m` ; au-delà → `1h`. L'interface applique cette règle, filtre la série affichée par `label`, et passe au pas supérieur si la réponse brute atteint `limit` ou si l'API renvoie `TOO_MANY_POINTS`.

### 5.4 Catalogue des métriques hôte

Une métrique absente signifie que la source n'est pas disponible ou que le calcul n'est pas défini pour ce cycle (voir METRICS_COLLECTION.md).

| Nom | Unité | Labels | Remarque |
|---|---|---|---|
| `cpu.cores` | `count` | — | Cœurs logiques |
| `cpu.percent` | `percent` | `core` : `all` ou index `0`… | Absent au premier cycle |
| `cpu.load1`, `cpu.load5`, `cpu.load15` | `processes` | — | `/proc/loadavg` |
| `memory.total`, `memory.used`, `memory.available` | `bytes` | — | `used = total − available` |
| `memory.cached` | `bytes` | — | `Cached + Buffers` |
| `swap.total`, `swap.used` | `bytes` | — | |
| `filesystem.total`, `filesystem.used`, `filesystem.available` | `bytes` | `mountpoint`, `device` | Partitions physiques montées. Plusieurs points de montage peuvent partager un même `device` (sous-volumes btrfs, montages *bind*) : un total ne compte chaque `device` qu'une fois |
| `filesystem.percent` | `percent` | `mountpoint`, `device` | |
| `disk.read_bytes`, `disk.write_bytes` | `bytes` | `device` | Compteurs cumulés |
| `disk.read_iops`, `disk.write_iops` | `iops` | `device` | Absent au premier cycle |
| `disk.read_latency_ms`, `disk.write_latency_ms` | `milliseconds` | `device` | Absent si aucune E/S dans l'intervalle |
| `network.rx_bytes`, `network.tx_bytes` | `bytes` | `interface` | Compteurs cumulés |
| `network.rx_packets`, `network.tx_packets` | `packets` | `interface` | |
| `network.errors`, `network.drops` | `count` | `interface` | Somme entrée + sortie |
| `network.rx_bytes_per_second`, `network.tx_bytes_per_second` | `bytes_per_second` | `interface` | Absent au premier cycle ou après remise à zéro |
| `network.interface` | `present` (valeur `1`) | `name`, `address`, `mac` | Échantillon d'inventaire : une ligne par adresse IP |
| `temperature.celsius` | `celsius` | `sensor` | Capteurs exposés par le noyau |

### 5.5 Catalogue des métriques conteneur

| Nom | Unité | Labels | Remarque |
|---|---|---|---|
| `cpu.percent` | `percent` | — | Peut dépasser 100 % (plusieurs cœurs) |
| `memory.usage_bytes` | `bytes` | — | Usage moins le cache inactif |
| `memory.limit_bytes`, `memory.available_bytes`, `memory.percent` | `bytes` / `percent` | — | Uniquement si une limite est connue |
| `network.rx_bytes`, `network.tx_bytes` | `bytes` | `interface` | |
| `network.rx_packets`, `network.tx_packets` | `packets` | `interface` | |
| `network.errors`, `network.drops` | `count` | `interface` | |
| `network.rx_bytes_per_second`, `network.tx_bytes_per_second` | `bytes_per_second` | `interface` | À partir du deuxième cycle |
| `block.read_bytes`, `block.write_bytes` | `bytes` | `device` (`majeur:mineur`) | Si Docker les fournit |
| `process.count` | `count` | — | Si Docker le fournit |
| `restart.count` | `count` | — | Émis aussi pour les conteneurs arrêtés |

## 6. Docker

### 6.1 Conteneurs

`GET /api/v1/containers[?includeRemoved=true|false]` → `{ "containers": [...], "nextCursor": null }`.

- Tri par nom, 500 conteneurs au plus. `includeRemoved=true` inclut les conteneurs disparus (`state = "removed"`). Tout autre paramètre → `400 VALIDATION_ERROR`.

DTO conteneur (liste et détail) :

```json
{
  "id": "manager-uuid",
  "dockerId": "4f1c…",
  "name": "web",
  "image": "nginx:stable",
  "state": "running",
  "status": "Up 3 hours",
  "createdAt": "2026-11-21T17:00:00Z",
  "startedAt": "2026-11-21T17:01:00Z",
  "uptimeSeconds": 10620,
  "restartCount": 0,
  "restartPolicy": "unless-stopped",
  "ports": [{ "privatePort": 80, "publicPort": 8080, "ip": "0.0.0.0", "type": "tcp" }],
  "networks": ["bridge"],
  "mounts": [{ "type": "bind", "source": "/srv/web", "destination": "/usr/share/nginx/html", "rw": false }],
  "lastObservedAt": "2026-11-21T20:58:00Z"
}
```

- `state` : état Docker (`running`, `exited`, `created`, `paused`, `restarting`, `dead`) ou `removed` lorsque le conteneur a disparu de Docker.
- `removedAt` n'est présent que pour un conteneur supprimé. `uptimeSeconds` vaut `null` si le conteneur n'est pas `running`.
- `ports[].publicPort` et `ports[].ip` sont absents si le port n'est pas publié. `mounts[].source` est un chemin de l'hôte : il est visible par l'administrateur, seul utilisateur de l'interface.
- Les variables d'environnement et les labels Docker ne sont jamais renvoyés : le proxy les supprime avant le Manager.

### 6.2 Événements

`GET /api/v1/events` et `GET /api/v1/containers/{containerID}/events`, avec `from`, `to`, `limit` comme au § 5.3 (`step` et `cursor` sont rejetés de la même façon) → `{ "events": [...], "nextCursor": null }`, du plus récent au plus ancien.

```json
{
  "id": "uuid",
  "containerId": "manager-uuid",
  "type": "container.state_changed",
  "severity": "info",
  "message": "Container state or restart count changed",
  "metadata": { "previousState": "running", "state": "exited" },
  "occurredAt": "2026-11-21T20:58:00Z"
}
```

Types émis par la réconciliation Docker, et uniquement ceux-ci :

| `type` | `severity` | Déclencheur | `metadata` |
|---|---|---|---|
| `container.discovered` | `info` | Conteneur vu pour la première fois | absent |
| `container.state_changed` | `info` | État modifié, compteur de redémarrages inchangé ou en baisse | `previousState`, `state` |
| `container.restarted` | `info` | Compteur de redémarrages en hausse | `previousState`, `state` |
| `container.removed` | `warning` | Conteneur absent de la liste Docker | absent |

Types émis par la collecte des services système (§ 4.7), sans `containerId` (`null`) :

| `type` | `severity` | Déclencheur | `metadata` |
|---|---|---|---|
| `service.failed` | `error` | Unité vue en `failed` alors qu'elle n'était pas enregistrée en échec | `unit`, `activeState`, `subState`, `result` |
| `service.recovered` | `info` | Unité enregistrée en échec revenue à `active` | `unit`, `activeState`, `subState`, `result` |

Une unité en échec n'est signalée qu'une fois, même après un redémarrage du Manager ou la purge de l'événement (table `failed_units`) ; une unité qui redémarre (`activating`) reste en échec jusqu'à son retour à `active` ; une unité disparue de l'inventaire est oubliée sans événement.

`severity` accepte `info`, `warning`, `error`, `critical` ; `critical` n'est pas émis actuellement. Aucun événement d'hôte ou de collecteur n'est persisté.

## 7. Versionnement

Les routes sont versionnées sous `/api/v1`. Un changement incompatible (champ supprimé ou renommé, sémantique modifiée) exige `/api/v2`. Ajouter un champ ou une valeur de `type` d'événement est compatible.

## 8. Erreurs

```json
{ "error": { "code": "VALIDATION_ERROR", "message": "from must precede to", "requestId": "host/abc-000001" } }
```

| Code | HTTP | Cas |
|---|---|---|
| `VALIDATION_ERROR` | 400 | Corps, paramètre de requête ou de chemin invalide |
| `INVALID_CREDENTIALS` | 401 | Identifiants ou mot de passe actuel incorrects |
| `SESSION_NOT_FOUND` | 401 | Session absente, expirée ou révoquée |
| `CROSS_ORIGIN_REQUEST` | 403 | Contrôle d'origine refusé |
| `TOO_MANY_POINTS` | 400 | Historique agrégé dépassant `limit` |
| `NOT_FOUND` | 404 | Route, conteneur, compte ou unité inconnus |
| `METHOD_NOT_ALLOWED` | 405 | Méthode non prévue sur la route |
| `RATE_LIMITED` | 429 | Trop de tentatives de connexion |
| `AUTH_UNAVAILABLE` | 503 | Base indisponible au démarrage |
| `METRICS_UNAVAILABLE` | 500 | Lecture des métriques en échec |
| `EVENTS_UNAVAILABLE` | 500 | Lecture des événements en échec |
| `LOGOUT_FAILED` | 500 | Révocation de session en échec |
| `PASSWORD_CHANGE_FAILED` | 500 | Écriture du nouveau mot de passe en échec |
| `SETTINGS_UPDATE_FAILED` | 500 | Écriture des paramètres en échec |
| `FORBIDDEN` | 403 | Rôle insuffisant pour la route |
| `PASSWORD_CHANGE_REQUIRED` | 403 | Mot de passe initial à changer d'abord |
| `USERNAME_TAKEN` | 409 | Identifiant de compte déjà utilisé |
| `LAST_ADMIN` | 409 | Dernier administrateur actif à conserver |
| `OWN_PASSWORD_RESET` | 409 | Réinitialisation de son propre mot de passe |
| `USERS_UNAVAILABLE` | 500 | Lecture des comptes en échec |
| `ACCOUNT_UPDATE_FAILED` | 500 | Écriture d'un compte en échec |
| `HOSTNAME_MISMATCH` | 400 | Confirmation différente du nom de la machine |
| `POWER_ACTIONS_DISABLED` | 409 | Redémarrage et extinction désactivés par la configuration |
| `ACTION_IN_PROGRESS` | 409 | Une action est déjà en cours |
| `POWER_ACTION_REFUSED` | 503 | polkit ou logind refuse l'action |
| `POWER_ACTION_FAILED` | 500 | Demande d'alimentation impossible à transmettre |
| `NETWORK_NOT_ALLOWED` | 403 | Adresse cliente hors des réseaux autorisés |
| `POLICY_EXCLUDES_CURRENT_ADDRESS` | 409 | La politique exclurait l'adresse de l'administrateur |
| `SESSIONS_UNAVAILABLE` | 500 | Lecture des sessions en échec |
| `SESSION_REVOKE_FAILED` | 500 | Révocation de session en échec |
| `SECURITY_POLICY_UPDATE_FAILED` | 500 | Écriture de la politique en échec |
| `AUDIT_UNAVAILABLE` | 500 | Lecture du journal d'audit en échec |
| `INVALID_SECOND_FACTOR` | 401, 400 | Code TOTP ou de secours faux (401 à la connexion, 400 à la confirmation) |
| `LOGIN_CHALLENGE_EXPIRED` | 401 | Défi de connexion absent, expiré ou épuisé |
| `SECOND_FACTOR_REQUIRED` | 403 | Second facteur à enrôler d'abord (administrateur, politique) |
| `SECOND_FACTOR_UNAVAILABLE` | 503 | Clé maître absente : TOTP indisponible |
| `SECOND_FACTOR_ALREADY_ENABLED` | 409 | Second facteur déjà activé |
| `SECOND_FACTOR_NOT_ENABLED` | 409 | Aucun second facteur à régénérer ou désactiver |
| `NO_PENDING_ENROLLMENT` | 409 | Aucun enrôlement TOTP en attente |
| `SECOND_FACTOR_NOT_ENROLLED` | 409 | Exiger le second facteur sans en avoir soi-même |
| `SECOND_FACTOR_UPDATE_FAILED` | 500 | Écriture du second facteur en échec |
| `JOURNAL_UNAVAILABLE` | 503 | `journalctl` absent : journal système indisponible |
| `JOURNAL_READ_FAILED` | 500 | Lecture du journal système en échec |
| `SYSTEMD_UNAVAILABLE` | 503 | systemd injoignable ou aucun inventaire des unités |
| `SYSTEMD_READ_FAILED` | 500 | Lecture d'une unité systemd en échec |
| `UNIT_NOT_MANAGEABLE` | 409 | Unité hors de la liste blanche `systemd_manageable_units` |
| `UNIT_ACTION_RATE_LIMITED` | 429 | Plus de 30 actions sur les unités dans la dernière minute |
| `UNIT_ACTIONS_UNAVAILABLE` | 503 | Actions sur les unités indisponibles (base de données absente, Manager en arrêt) |
| `UNIT_ACTION_FAILED` | 500 | Demande d'action impossible à enregistrer (audit en échec) |
| `ALERT_ALREADY_RESOLVED` | 409 | Acquittement d'une alerte résolue |
| `DEFAULT_RULE` | 409 | Suppression d'une règle d'alerte par défaut |
| `TOO_MANY_RULES` | 409 | Plus de 200 règles d'alerte |
| `TOO_MANY_SILENCES` | 409 | Plus de 100 silences en vigueur |
| `TOO_MANY_CHANNELS` | 409 | Plus de 20 canaux de notification |
| `SECRETS_UNAVAILABLE` | 503 | Clé maître absente : secrets des canaux indisponibles |
| `NOTIFICATION_FAILED` | 502 | Envoi de test en échec (code et statut seulement) |
| `ALERTS_UNAVAILABLE` | 503 | Moteur d'alertes arrêté (base de données indisponible) |
| `SCAN_IN_PROGRESS` | 409 | Un scan de vulnérabilités est déjà en cours |
| `SCAN_RATE_LIMITED` | 429 | Scan manuel moins de 5 min après le précédent |
| `VULNERABILITIES_UNAVAILABLE` | 503 | Module des vulnérabilités arrêté (base de données indisponible) |
| `VULNERABILITIES_FAILED` | 500 | Lecture ou lancement des scans en échec |
| `ALERTS_FAILED` | 500 | Lecture ou écriture des alertes en échec |
| `INTERNAL_ERROR` | 500 | Autre erreur serveur |

Aucune trace de pile, chemin local, secret ou réponse Docker brute dans les erreurs. `requestId` correspond au champ `request_id` des journaux du Manager.

## 9. Évolutions spécifiées, non implémentées

Ces contrats sont arrêtés pour guider l'implémentation ; tant qu'ils ne sont pas livrés, les paramètres correspondants restent rejetés.

### 9.1 Pagination par curseur

Pour `GET /api/v1/containers` et les événements : `cursor` opaque (base64url de `(clé de tri, id)`), `nextCursor` non nul tant qu'il reste des résultats. L'ordre doit être stable à égalité d'horodatage (départage par `id`).
