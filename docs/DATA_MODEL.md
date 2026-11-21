# Modèle de données initial — Homelab Manager

Version 1.1 — 20 novembre 2026

Modèle logique à traduire en schémas Ent et migrations PostgreSQL. Les champs et index exacts seront affinés avec les contrats API.

## Entités principales

### AdminCredential
- `id`
- `password_hash`
- `created_at`
- `updated_at`
- Ne stocke jamais le mot de passe en clair.

### Agent
- `id`
- `name`
- `hostname`
- `agent_version`
- `os`
- `architecture`
- `status`
- `last_seen_at`
- `created_at`
- `revoked_at`
- `capabilities` (JSON structuré ou relation normalisée selon les requêtes prévues)

### HostInventory
- `id`
- `agent_id`
- `collected_at`
- `kernel_version`
- `uptime_seconds`
- `cpu_info`
- `memory_total_bytes`
- `disks` / volumes
- `network_interfaces`

### MetricSample
- `id`
- `agent_id`
- `metric_name`
- `value`
- `unit`
- `labels`
- `recorded_at`

Prévoir des index par agent, métrique et temps. Définir une stratégie de rétention et de purge ; ne pas conserver indéfiniment des échantillons bruts sans limite.

### Service
- `id`
- `agent_id`
- `name`
- `display_name`
- `state`
- `enabled`
- `observed_at`

### Container
- `id`
- `agent_id`
- `runtime`
- `runtime_id`
- `name`
- `image`
- `state`
- `cpu_usage`
- `memory_usage_bytes`
- `observed_at`

### Operation
- `id`
- `agent_id`
- `requested_by`
- `action`
- `parameters` (JSON validé par action)
- `status`
- `created_at`
- `started_at`
- `finished_at`
- `result`
- `error_code`

### Event
- `id`
- `agent_id` nullable pour événements Manager
- `type`
- `severity`
- `message`
- `metadata`
- `occurred_at`

### Alert
- `id`
- `rule_id`
- `agent_id` nullable
- `severity`
- `status`
- `summary`
- `created_at`
- `acknowledged_at`

### AuditLog
- `id`
- `actor`
- `target_type`
- `target_id`
- `action`
- `request_id`
- `outcome`
- `created_at`

## Règles de persistance

- UUID comme identifiants d'API.
- Clés étrangères et contraintes d'intégrité.
- Index adaptés aux filtres réellement exposés.
- Migrations versionnées et reproductibles.
- Horodatages UTC.
- Ne pas utiliser de JSON libre pour les données nécessitant filtres, contraintes ou relations fréquentes.
- Les schémas Ent sont des détails d'infrastructure ; les DTO API et objets de domaine restent indépendants.
