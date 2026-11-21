# Contrats API — Homelab Manager

Version 1.1 — 20 novembre 2026

## 1. Principes

- Préfixe REST : `/api/v1`.
- JSON UTF-8.
- DTO explicites ; ne jamais exposer directement les modèles Ent.
- Identifiants stables (UUID recommandé).
- Horodatages ISO 8601 UTC.
- Pagination pour les collections historiques.
- Validation serveur systématique.
- Erreurs au format uniforme.
- Version de protocole explicite pour WebSocket Agent.

## 2. Routes REST initiales

### Authentification
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/logout`
- `GET /api/v1/auth/session`
- `POST /api/v1/auth/password`

### Tableau de bord et inventaire
- `GET /api/v1/dashboard`
- `GET /api/v1/agents`
- `GET /api/v1/agents/{agentID}`
- `POST /api/v1/agents/enrollment-tokens`
- `POST /api/v1/agents/{agentID}/revoke`
- `GET /api/v1/hosts`
- `GET /api/v1/hosts/{hostID}`
- `GET /api/v1/hosts/{hostID}/metrics`
- `GET /api/v1/hosts/{hostID}/services`
- `GET /api/v1/hosts/{hostID}/containers`

### Opérations
- `GET /api/v1/operations`
- `POST /api/v1/operations`
- `GET /api/v1/operations/{operationID}`
- `POST /api/v1/operations/{operationID}/cancel` (uniquement si l'action le permet)

### Alertes et événements
- `GET /api/v1/events`
- `GET /api/v1/alerts`
- `POST /api/v1/alerts/{alertID}/acknowledge`

### Paramètres
- `GET /api/v1/settings`
- `PATCH /api/v1/settings`

Les routes sont un contrat initial : préciser chaque schéma request/response avant d'implémenter la route correspondante.

## 3. Format d'erreur

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "The request is invalid.",
    "requestId": "stable-request-id",
    "details": [
      { "field": "name", "reason": "required" }
    ]
  }
}
```

Ne pas exposer de stack trace, chemin local, secret ou détail interne dans les réponses client.

## 4. Format d'opération

```json
{
  "id": "uuid",
  "agentId": "uuid",
  "action": "service.restart",
  "status": "queued",
  "createdAt": "2026-11-20T12:00:00Z",
  "startedAt": null,
  "finishedAt": null,
  "result": null
}
```

Statuts autorisés : `queued`, `running`, `succeeded`, `failed`, `timed_out`, `cancelled`.

## 5. WebSocket Agent

- Endpoint proposé : `/api/v1/agent/stream`.
- Connexion sortante Agent → Manager, protégée par TLS et authentification Agent.
- Enveloppe de message versionnée :

```json
{
  "protocolVersion": 1,
  "messageId": "uuid",
  "type": "heartbeat",
  "sentAt": "2026-11-20T12:00:00Z",
  "payload": {}
}
```

Types de messages initiaux :
- `agent.hello`
- `manager.hello`
- `heartbeat`
- `metrics.batch`
- `inventory.report`
- `event.report`
- `task.dispatch`
- `task.accepted`
- `task.result`
- `agent.capabilities`

Exigences :
- valider type, version, taille et payload ;
- rejeter les types inconnus ;
- limiter le nombre de connexions et le débit ;
- détecter les heartbeats expirés ;
- gérer les doublons et identifiants de messages ;
- ne jamais traiter un payload comme une commande shell ;
- ne pas considérer une tâche envoyée comme exécutée avant réception d'un résultat authentifié.
