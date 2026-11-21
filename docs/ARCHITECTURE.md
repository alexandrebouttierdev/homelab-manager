# Architecture technique — Homelab Manager

Version 1.1 — 20 novembre 2026

## 1. Vue logique

```text
Navigateur (React / TypeScript)
          |
       HTTPS / REST
          |
Manager Go
  |-- Delivery HTTP : Chi v5
  |-- Delivery WebSocket : coder/websocket
  |-- Usecases
  |-- Domain
  |-- Ports (repositories, agent gateway, clock, ...)
  |-- Adapters : Ent / PostgreSQL
  |
PostgreSQL dédié

Agents Go (plusieurs hôtes)
  |-- collecte système et capacités
  |-- exécution d'actions autorisées uniquement
  |-- connexion sortante WebSocket TLS vers Manager
  |-- service systemd
```

## 2. Backend Manager

### Routeur HTTP : Chi v5

Dépendance validée :

```bash
go get github.com/go-chi/chi/v5
```

Import :

```go
import "github.com/go-chi/chi/v5"
```

Chi est le routeur HTTP de la couche Delivery. Il s'appuie sur `net/http` et ne doit pas devenir le cœur de l'application.

Responsabilités de la couche HTTP :
- routage et versionnement de l'API ;
- middleware HTTP (request ID, récupération contrôlée des panics, logs, timeout, authentification, autorisation, limites de taille) ;
- décodage et validation des DTO ;
- conversion des erreurs applicatives vers des réponses HTTP ;
- appel des usecases.

Les handlers ne contiennent aucune logique métier et ne manipulent pas directement Ent.

### Autres dépendances validées

| Besoin | Technologie |
|---|---|
| Routeur HTTP | Chi v5 |
| ORM | Ent |
| PostgreSQL | PostgreSQL natif |
| WebSocket | `github.com/coder/websocket` |
| Configuration | Viper |
| Logs structurés | Zap |
| Frontend | React + TypeScript + Vite |
| Formulaires | React Hook Form + Zod |
| Graphiques | Apache ECharts |
| Tests UI | Vitest + React Testing Library |

Ne pas ajouter un framework web complet au-dessus de Chi sans décision d'architecture.

## 3. Communication hybride

### REST/HTTPS
Utilisé pour :
- authentification de l'administrateur ;
- consultation des machines, agents, inventaires, métriques et événements ;
- gestion des paramètres ;
- création, consultation et suivi des opérations ;
- enrôlement, révocation et administration des agents.

### WebSocket TLS
Utilisé pour :
- connexion persistante initiée par chaque Agent vers le Manager ;
- heartbeat et état de présence ;
- remontée de métriques et événements ;
- réception des tâches autorisées et transmission de leurs résultats ;
- négociation des capacités supportées par l'Agent.

L'Agent initie la connexion sortante. Le Manager n'a pas besoin d'ouvrir un port entrant sur chaque machine supervisée.

Les messages WebSocket doivent être versionnés, typés, bornés en taille et validés. La reconnexion doit utiliser un délai progressif avec jitter et respecter l'annulation du contexte.

## 4. Agent Go

Responsabilités :
- s'identifier auprès du Manager ;
- annoncer version, OS, architecture et capacités ;
- collecter les métriques et inventaires autorisés ;
- envoyer heartbeat, métriques et événements ;
- recevoir uniquement des tâches reconnues ;
- vérifier localement que l'action est autorisée ;
- appliquer des timeouts et limites de ressources ;
- renvoyer résultat structuré et statut.

L'Agent ne doit jamais transformer une requête du Manager en commande shell libre. Les actions sont des opérations nommées, avec paramètres structurés et validation stricte.

## 5. Architecture hexagonale

Organisation recommandée :

- `domain/` : entités métier, value objects, règles et erreurs métier.
- `usecase/` : orchestration et cas d'utilisation.
- `ports/` : interfaces nécessaires aux usecases.
- `adapters/` : PostgreSQL/Ent, horloge, passerelle Agent, système de fichiers si nécessaire.
- `delivery/http/` : handlers et middleware Chi.
- `delivery/ws/` : protocole WebSocket.
- `cmd/manager/` et `cmd/agent/` : points d'entrée.

Le domaine et les usecases ne doivent pas importer Chi, Ent ou les packages PostgreSQL.

## 6. PostgreSQL

- Installation native dédiée sur Debian 13.
- Base dédiée `homelab_manager`.
- Rôle PostgreSQL applicatif dédié, avec privilèges minimaux sur cette base.
- Connexion locale uniquement par défaut ; aucun accès réseau PostgreSQL nécessaire au frontend ou aux Agents.
- Migrations versionnées et testées.
- Ent est l'accès ORM ; les types générés Ent restent dans l'adaptateur de persistance.

## 7. Frontend

- React + TypeScript + Vite.
- Client API centralisé, gestion uniforme des erreurs et de l'authentification.
- Zod pour valider les réponses et entrées pertinentes côté client.
- React Hook Form pour les formulaires.
- ECharts pour les séries temporelles et visualisations.
- WebSocket navigateur pour les mises à jour live ; REST reste la source de vérité pour les lectures et mutations durables.
- États explicites : chargement, vide, erreur, données périmées, hors ligne.

## 8. Arborescence monorepo proposée

```text
homelab-manager/
├── CLAUDE.md
├── README.md
├── go.work
├── backend/
│   ├── cmd/manager/
│   ├── internal/
│   │   ├── domain/
│   │   ├── usecase/
│   │   ├── ports/
│   │   ├── adapters/postgres/
│   │   └── delivery/
│   │       ├── http/       # Chi v5
│   │       └── websocket/
│   ├── ent/schema/
│   ├── migrations/
│   └── go.mod
├── agent/
│   ├── cmd/agent/
│   ├── internal/
│   │   ├── collector/
│   │   ├── executor/
│   │   ├── transport/
│   │   └── capabilities/
│   └── go.mod
├── frontend/
│   ├── src/
│   │   ├── app/
│   │   ├── features/
│   │   ├── components/
│   │   ├── lib/api/
│   │   └── schemas/
│   └── package.json
├── deployments/
│   ├── systemd/
│   └── config/
└── docs/
```

Cette arborescence est une convention de départ ; ne pas créer artificiellement des couches vides. Garder les modules Go séparés si cela simplifie réellement les dépendances et les builds.
