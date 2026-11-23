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
- Toutes les routes sauf `/health`, `/api/v1/health`, `/api/v1/auth/login` et `/api/v1/auth/session` exigent une session.
- Les messages d'erreur de l'API sont techniques et en anglais ; l'interface affiche ses propres messages, en français, à partir du `code`.

## 2. Routes

| Méthode et chemin | Session | Réponse |
|---|---|---|
| `GET /health` | non | Processus HTTP vivant (§ 4.1) |
| `GET /api/v1/health` | non | Santé détaillée (§ 4.1) |
| `POST /api/v1/auth/login` | non | Ouvre une session (§ 3) |
| `GET /api/v1/auth/session` | cookie | Utilisateur courant (§ 3) |
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
| `GET /api/v1/machine/power` | admin | Actions d'alimentation disponibles (§ 4.5) |
| `POST /api/v1/machine/power/{action}` | admin | Redémarre ou éteint la machine (§ 4.5) |

Toutes les routes de supervision sont en lecture seule ; les seules écritures sont l'authentification, `PUT /api/v1/settings/retention`, la gestion des comptes et les demandes d'alimentation de la machine. La colonne « Session » indique `admin` pour les routes réservées au rôle administrateur (§ 3.1). Une route inconnue sous `/api` répond `404 NOT_FOUND`, une méthode non prévue `405 METHOD_NOT_ALLOWED`, toujours au format d'erreur du § 8. Il n'existe aucune route d'Agent, d'hôte distant, de journaux, ni d'action Docker (création, suppression, démarrage, arrêt, restart, exec).

### Paramètres de chemin

- `containerID` : UUID Manager (`8-4-4-4-12` hexadécimal minuscule) ou ID Docker de 12 à 64 caractères hexadécimaux minuscules. Sinon `400 VALIDATION_ERROR` ; inconnu → `404 NOT_FOUND`.
- `userID` : UUID Manager en minuscules (`8-4-4-4-12`). Sinon `400 VALIDATION_ERROR` ; inconnu → `404 NOT_FOUND`.
- `action` : `reboot` ou `poweroff`. Sinon `400 VALIDATION_ERROR`.
- `metricName` : 64 caractères au plus, motif `^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$` (ex. `cpu.percent`). Sinon `400 VALIDATION_ERROR`. Un nom valide mais jamais collecté renvoie une liste vide.

## 3. Authentification et sessions

**Connexion** — `POST /api/v1/auth/login` avec `{ "username": "...", "password": "..." }`.

- `username` : espaces de début et fin retirés, 1 à 255 octets. `password` : 1 à 1 024 octets. Aucune longueur minimale à la connexion (le compte par défaut est `admin` / `admin`).
- Succès `200` : `{ "user": { "id": "uuid", "username": "admin", "displayName": "admin", "role": "admin", "mustChangePassword": false } }` et cookie de session. Un compte désactivé reçoit `401 INVALID_CREDENTIALS`, comme un mot de passe faux.
- Échec : `401 INVALID_CREDENTIALS`, identique que l'utilisateur existe ou non ; le temps de réponse est équivalent (une vérification Argon2id est faite dans les deux cas).
- Limitation : 10 tentatives par minute et par adresse IP source, au-delà `429 RATE_LIMITED` (fenêtre fixe d'une minute, au plus 10 000 adresses suivies).
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

**Rôles** — `viewer` : consultation et son propre mot de passe. `operator` : idem pour l'instant (actions sur les services et sauvegardes dans les phases suivantes). `admin` : idem, plus `PUT /api/v1/settings/retention` et les routes `/api/v1/users`. Un rôle insuffisant reçoit `403 FORBIDDEN`.

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

- `GET /api/v1/security/policy` (admin) : `200 { "idleTimeoutMinutes": 60, "allowedNetworks": [], "currentAddress": "192.168.1.20" }`. `currentAddress` est l'adresse vue par le Manager pour la requête.
- `PUT /api/v1/security/policy` (admin) avec `{ "idleTimeoutMinutes": 60, "allowedNetworks": ["192.168.1.0/24"] }`, les deux champs obligatoires : `idleTimeoutMinutes` de 5 à 720 ; `allowedNetworks` 32 préfixes CIDR au plus, stockés masqués ; liste vide = toutes les adresses. Succès `200` (même objet que `GET`), avec audit (`security_policy`, `local`, `update`). Une politique qui exclurait `currentAddress` : `409 POLICY_EXCLUDES_CURRENT_ADDRESS`.
- **Réseaux autorisés** — avec une liste non vide, toute requête d'une adresse hors liste reçoit `403 NETWORK_NOT_ALLOWED`, interface comprise, sauf `GET /health`. L'adresse est celle de la connexion TCP (`RemoteAddr`) : derrière un proxy inverse, c'est celle du proxy ; aucun en-tête `X-Forwarded-For` n'est pris en compte.

### 3.4 Journal d'audit

- `GET /api/v1/audit` (admin) : `200 { "entries": [Entrée] }`, les plus récentes d'abord. Filtres facultatifs : `actor`, `targetType`, `action`, `outcome` (égalité exacte, 255 octets au plus), `from` et `to` (RFC 3339, intervalle `[from, to)`, `from` avant `to`), `limit` (1 à 1 000, défaut 200). Sinon `400 VALIDATION_ERROR`.

```json
{ "id": "uuid", "actor": "alice", "targetType": "route", "targetId": "GET /api/v1/users",
  "action": "access", "outcome": "denied", "requestId": "host/abc-000012", "createdAt": "…" }
```

- `GET /api/v1/audit/export?format=csv|json` (admin), mêmes filtres, 10 000 lignes au plus (défaut et maximum) : fichier joint `audit-<horodatage>.csv` ou `.json`. Le CSV a l'en-tête `createdAt,actor,targetType,targetId,action,outcome,requestId` ; une valeur commençant par `=`, `+`, `-`, `@`, une tabulation ou un retour chariot est préfixée d'une apostrophe (pas de formule dans un tableur).
- Résultats tracés : `success`, `denied` (rôle insuffisant : cible `route`, `METHODE chemin`, action `access` ; dernier administrateur protégé : cible `user`), `requested` et `failure` (alimentation, § 4.5).

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

`severity` accepte `info`, `warning`, `error`, `critical` ; seuls `info` et `warning` sont émis actuellement. Aucun événement d'hôte ou de collecteur n'est persisté.

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
| `NOT_FOUND` | 404 | Route, conteneur ou compte inconnu |
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
| `INTERNAL_ERROR` | 500 | Autre erreur serveur |

Aucune trace de pile, chemin local, secret ou réponse Docker brute dans les erreurs. `requestId` correspond au champ `request_id` des journaux du Manager.

## 9. Évolutions spécifiées, non implémentées

Ces contrats sont arrêtés pour guider l'implémentation ; tant qu'ils ne sont pas livrés, les paramètres correspondants restent rejetés.

### 9.1 Pagination par curseur

Pour `GET /api/v1/containers` et les événements : `cursor` opaque (base64url de `(clé de tri, id)`), `nextCursor` non nul tant qu'il reste des résultats. L'ordre doit être stable à égalité d'horodatage (départage par `id`).
