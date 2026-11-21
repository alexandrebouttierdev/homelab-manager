# Contrats API — Homelab Manager

Version 2.0 — 21 novembre 2026

L'API v2.0 est implémentée pour les routes de consultation et l'authentification ci-dessous. Les historiques sont bruts ; l'agrégation et la pagination par curseur restent non disponibles.

## 1. Principes communs

- API REST JSON UTF-8 sous `/api/v1`, same-origin en production.
- DTO explicites ; ne jamais exposer les entités Ent ni les réponses brutes Docker.
- UUID côté Manager ; ID Docker opaque uniquement comme identifiant externe de conteneur.
- Horodatages RFC 3339 en UTC.
- Authentification par session cookie ; mutations protégées contre CSRF et origines externes.
- Historique borné par `from`, `to` et `limit`.
- Une donnée inconnue/indisponible est `null` ou absente selon le DTO, jamais remplacée par zéro.
- Les noms de machine et IDs de conteneur ne représentent qu'un seul hôte local.

## 2. Routes

### Santé et authentification
- `GET /health` — disponibilité du processus HTTP.
- `GET /api/v1/health` — santé du Manager et statut des dépendances/collecteurs sans secret.
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/logout`
- `GET /api/v1/auth/session`
- `POST /api/v1/auth/password`

### Dashboard et hôte
- `GET /api/v1/dashboard`
- `GET /api/v1/host`
- `GET /api/v1/host/metrics/current`
- `GET /api/v1/host/metrics/{metricName}`
- `GET /api/v1/host/temperatures`

Métriques hôte supportées selon disponibilité : CPU global/par cœur/charge, mémoire/swap, systèmes de fichiers, activité bloc, interfaces réseau et températures exposées localement.

### Docker
- `GET /api/v1/containers`
- `GET /api/v1/containers/{containerID}`
- `GET /api/v1/containers/{containerID}/metrics/current`
- `GET /api/v1/containers/{containerID}/metrics/{metricName}`
- `GET /api/v1/containers/{containerID}/events`

Les listes comprennent les conteneurs arrêtés si Docker les retourne. Les routes sont en lecture seule. Pas de logs dans le périmètre initial, ni de création/suppression/démarrage/arrêt/restart/exec.

### Événements
- `GET /api/v1/events` — changements d'état détectés lors de la réconciliation des conteneurs.

### Paramètres effectifs
- `GET /api/v1/settings` — expose les valeurs non secrètes actives (fréquences, rétention, statut Docker et capacités de collecte). Les valeurs sont configurées par fichier ou variables d'environnement, pas modifiées par l'API.

Il n'y a pas de route Agent, d'opération distante, de flux WebSocket Manager-Agent, ni de route de gestion de plusieurs hôtes.

## 3. Filtres temporels et pagination

Pour les historiques et événements :

- `from` et `to` : bornes RFC 3339 UTC requises ; intervalle semi-ouvert `[from, to)`.
- `from` doit précéder `to` ; l'intervalle ne peut dépasser 365 jours. Les échantillons antérieurs à la rétention configurée sont supprimés.
- `step` omis ou `raw` uniquement. Les valeurs `1m`, `5m` et `1h` sont rejetées jusqu'à l'implémentation des agrégations.
- `limit` facultatif, défaut 500, maximum 2000.
- `cursor` n'est pas encore supporté et toute valeur fournie est rejetée.
- Les routes de liste conteneurs et événements sont actuellement plafonnées à 500 résultats sans curseur.

Le serveur rejette les filtres invalides au lieu de choisir silencieusement des valeurs. Les échantillons sont renvoyés dans l'ordre chronologique ; les événements sont renvoyés du plus récent au plus ancien. Voir [DATA_MODEL.md](./DATA_MODEL.md).

## 4. DTO principaux

### Hôte

```json
{
  "hostname": "nas",
  "os": { "distribution": "Debian GNU/Linux", "version": "13" },
  "kernelVersion": "6.x",
  "bootTime": "2026-11-20T08:00:00Z",
  "uptimeSeconds": 129600,
  "collector": { "status": "ok", "lastSuccessAt": "2026-11-21T20:58:00Z" }
}
```

`distribution`, `version`, `kernelVersion`, `bootTime` ou uptime peuvent être vides/nuls si la source locale n'est pas disponible. L'uptime est calculé depuis l'heure de démarrage.

### Santé et paramètres

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

Le endpoint santé retourne HTTP 200 lorsque le processus répond, même si une dépendance est dégradée ; le champ `status` rend cet état explicite. `GET /api/v1/settings` retourne les fréquences et rétention effectives ainsi que le statut des collecteurs, sans secret, chemin de socket ni identifiant sensible. Les réglages se modifient dans la configuration locale puis nécessitent le redémarrage du service.

### Échantillon de métrique

```json
{
  "name": "cpu.percent",
  "value": 21.4,
  "unit": "percent",
  "labels": { "core": "all" },
  "recordedAt": "2026-11-21T20:58:00Z"
}
```

Un historique retourne `{ "samples": [...], "nextCursor": null, "resolution": "raw" }`.

### Conteneur

```json
{
  "id": "manager-uuid",
  "dockerId": "sha256:...",
  "name": "web",
  "image": "nginx:stable",
  "state": "running",
  "status": "Up 3 hours",
  "createdAt": "2026-11-21T17:00:00Z",
  "startedAt": "2026-11-21T17:01:00Z",
  "uptimeSeconds": 10620,
  "restartCount": 0,
  "ports": [],
  "networks": [],
  "mounts": [],
  "lastObservedAt": "2026-11-21T20:58:00Z"
}
```

Les champs sensibles de l'inspection Docker, notamment les variables d'environnement, ne sont jamais renvoyés. Valeurs indisponibles sont nulles ou absentes.

### Dashboard

Le dashboard retourne l'identité et la fraîcheur de l'hôte, métriques système récentes, nombre total et nombre par état de conteneur, ainsi qu'un résumé de disponibilité des collecteurs. Les valeurs sont cohérentes avec les endpoints `host` et `containers` et portent leur horodatage.

```json
{
  "host": {
    "hostname": "nas",
    "os": { "distribution": "Debian GNU/Linux", "version": "13" },
    "kernelVersion": "6.x",
    "bootTime": "2026-11-20T08:00:00Z",
    "uptimeSeconds": 129600,
    "collector": { "status": "ok", "lastSuccessAt": "2026-11-21T20:58:00Z" }
  },
  "containers": { "total": 4, "running": 3, "stopped": 1, "other": 0 },
  "metrics": [
    { "name": "cpu.percent", "value": 21.4, "unit": "percent", "recordedAt": "2026-11-21T20:58:00Z" },
    { "name": "memory.used", "value": 8589934592, "unit": "bytes", "recordedAt": "2026-11-21T20:58:00Z" }
  ],
  "collectors": {
    "system": { "status": "ok", "lastSuccessAt": "2026-11-21T20:58:00Z" },
    "docker": { "status": "ok", "lastSuccessAt": "2026-11-21T20:58:00Z" }
  }
}
```

### Conteneurs et événements

`GET /api/v1/containers` retourne `{ "containers": [...], "nextCursor": null }` avec un maximum de 500 résultats ; `includeRemoved=true` inclut les entrées supprimées. Le détail retourne le DTO conteneur présenté plus haut. Une lecture de métriques courantes retourne `{ "samples": [...], "lastSuccessAt": "...", "status": "ok" }`. Les événements utilisent :

```json
{
  "id": "uuid",
  "containerId": "manager-uuid",
  "type": "container.started",
  "severity": "info",
  "message": "Container started",
  "occurredAt": "2026-11-21T20:58:00Z"
}
```

Les événements émis actuellement concernent les conteneurs ; les événements hôte/collecteur ne sont pas encore persistés. Ils sont ordonnés du plus récent au plus ancien. La réponse inclut actuellement `nextCursor: null` ; une pagination future devra définir un curseur stable.

Les noms de métriques sont stables et les valeurs ont une unité explicite :

- Hôte : `cpu.percent`, `cpu.load1`, `cpu.load5`, `cpu.load15`, `memory.total`, `memory.used`, `memory.available`, `memory.cached`, `swap.total`, `swap.used`, `filesystem.total`, `filesystem.used`, `filesystem.available`, `filesystem.percent`, `disk.read_bytes`, `disk.write_bytes`, `disk.read_iops`, `disk.write_iops`, `disk.read_latency_ms`, `disk.write_latency_ms`, `network.rx_bytes`, `network.tx_bytes`, `network.rx_bytes_per_second`, `network.tx_bytes_per_second`, `network.rx_packets`, `network.tx_packets`, `network.errors`, `network.drops`, `temperature.celsius`.
- Conteneur : `cpu.percent`, `memory.usage_bytes`, `memory.limit_bytes`, `memory.available_bytes`, `memory.percent`, `network.rx_bytes`, `network.tx_bytes`, `network.rx_bytes_per_second`, `network.tx_bytes_per_second`, `network.rx_packets`, `network.tx_packets`, `network.errors`, `network.drops`, `block.read_bytes`, `block.write_bytes`, `process.count`, `restart.count`.

Les labels distinguent cœur, mountpoint, device, interface, capteur ou identifiant de conteneur selon la série. Métriques indisponibles pour une source/cgroup sont omises, avec état de fraîcheur explicite.

## 5. Authentification et erreurs

La connexion accepte `{ "username": "...", "password": "..." }`, crée une session cookie HTTP-only et retourne les informations publiques de l'utilisateur. La lecture de session retourne `{ "user": { "id": "...", "username": "..." } }`. La déconnexion révoque la session.

Format uniforme :

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "The request is invalid.",
    "requestId": "stable-request-id"
  }
}
```

Codes utilisés : `VALIDATION_ERROR`, `AUTH_UNAVAILABLE`, `INVALID_CREDENTIALS`, `SESSION_NOT_FOUND`, `CROSS_ORIGIN_REQUEST`, `RATE_LIMITED`, `NOT_FOUND`, `METRICS_UNAVAILABLE`, `EVENTS_UNAVAILABLE`, `INTERNAL_ERROR`. Aucun stack trace, chemin local, secret ou payload Docker brut dans la réponse.

## 6. Versionnement

Les routes sont versionnées sous `/api/v1`. Les changements incompatibles nécessitent un nouveau préfixe. Cette spécification cible une API mono-machine et ne décrit aucun protocole de collecte distant.
