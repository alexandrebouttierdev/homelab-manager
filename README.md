# Homelab Manager

Homelab Manager centralise la supervision d'un homelab composé d'un Manager Go et de plusieurs Agents Go. Le produit est conçu pour un déploiement local natif sur Debian, avec PostgreSQL dédié.

## Structure

- `backend/` : Manager Go, API HTTP Chi, Ent/PostgreSQL, WebSocket.
- `agent/` : Agent Go indépendant avec collecte locale et reconnexion.
- `frontend/` : interface React/TypeScript/Vite.
- `docs/` : spécifications produit, architecture, API, modèle de données, sécurité et tests.
- `deployments/` : configurations et unités systemd.
- `scripts/` : scripts de développement et vérification.

Les documents de [docs/](docs/) constituent la référence fonctionnelle et technique.

## Prérequis

- Go 1.26 ou supérieur
- Node.js 22 ou supérieur
- PostgreSQL 17 recommandé

## Configuration

Copier [.env.example](.env.example) vers `.env` puis renseigner uniquement les secrets propres à l'environnement.

Variables Manager :

- `HOMELAB_HTTP_ADDRESS` : adresse d'écoute HTTP, par défaut `:8080`.
- `HOMELAB_DATABASE_URL` : chaîne PostgreSQL dédiée.

Variables Agent :

- `HOMELAB_AGENT_MANAGER_URL` : URL HTTPS du Manager.
- `HOMELAB_AGENT_AGENT_TOKEN` : jeton d'enrôlement conservé avec des permissions restreintes.
- `HOMELAB_AGENT_COLLECT_INTERVAL_SECONDS` : intervalle de collecte, minimum 5 secondes.

Ne jamais enregistrer de secret dans Git.

## Lancement en développement

Option PostgreSQL avec Docker :

```bash
docker compose up -d postgres
```

Démarrer le Manager :

```bash
cd backend
go run ./cmd/manager
```

Le Manager répond sur `/health` et `/api/v1/health`.

Démarrer le frontend :

```bash
cd frontend
npm install
npm run dev
```

Démarrer l'Agent :

```bash
cd agent
HOMELAB_AGENT_MANAGER_URL=https://manager.example.internal \
HOMELAB_AGENT_AGENT_TOKEN=<token> \
go run ./cmd/agent
```

## Migrations PostgreSQL

La migration SQL initiale se trouve dans [backend/migrations/001_initial.sql](backend/migrations/001_initial.sql). Elle doit être appliquée à une base PostgreSQL dédiée avant l'intégration complète des repositories Ent.

## Tests et vérification

```bash
scripts/verify.sh
```

Le script vérifie le formatage Go, les tests/vérifications Go, ESLint, le typage strict, les tests Vitest et le build Vite.

## Déploiement Debian

Les unités systemd sont préparées dans [deployments/systemd/](deployments/systemd/). Elles utilisent des utilisateurs système dédiés, `NoNewPrivileges=true` et des fichiers d'environnement séparés.

PostgreSQL reste natif sur Debian. Docker est facultatif et réservé au développement ou aux tests PostgreSQL.
