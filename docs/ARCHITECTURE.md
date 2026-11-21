# Architecture technique — Homelab Manager

Version 2.0 — 21 novembre 2026

## 1. Vue logique

```text
Navigateur React / TypeScript
          | HTTPS / REST, même origine
          v
Backend Go
  |-- Delivery HTTP : Chi v5
  |-- Usecases
  |-- Domain et ports
  |-- Collecteur système Linux
  |-- Service de planification et rétention
  |-- Adaptateur Docker : API locale via proxy en liste blanche
  |-- Adaptateur Ent / PostgreSQL
          |
          +---- Unix socket local ---- Docker Engine
          |
          +---- PostgreSQL local dédié
```

Le périmètre est un hôte Linux unique : celui qui exécute le Manager. Aucun collecteur, transport ou modèle n'est conçu pour un hôte distant.

## 2. Stack et responsabilités

| Besoin | Technologie / règle |
|---|---|
| Langage Backend | Go |
| API HTTP | Chi v5 et `net/http` |
| ORM / persistance | Ent / PostgreSQL dédié |
| Configuration | Viper |
| Logs | Zap |
| Collecte système | Interfaces Linux locales (`/proc`, `/sys`) et bibliothèques Go adaptées |
| Collecte Docker | Docker Engine API sur socket Unix local, derrière un port d'infrastructure |
| Frontend | React, TypeScript strict, Vite, React Router |
| Cache serveur UI | TanStack Query |
| Formulaires / validation UI | React Hook Form / Zod |
| Graphiques | Apache ECharts |
| Tests | `testing`, `httptest`, Vitest et React Testing Library |

Chi reste exclusivement dans Delivery/HTTP. Ent reste dans les adaptateurs de persistance. Le domaine ne dépend ni de Chi, ni d'Ent, ni de Docker, ni de PostgreSQL, ni de React.

## 3. Architecture hexagonale

- `domain/` : objets et règles métier pour l'hôte, les conteneurs, les mesures et événements.
- `usecase/` : orchestration des lectures, collectes, historiques et paramètres.
- `ports/` : interfaces minimales pour les collecteurs, repositories et horloge si nécessaire.
- `adapters/` : Linux, API Docker, PostgreSQL/Ent et système de fichiers.
- `delivery/http/` : handlers Chi, DTO, middleware et validation HTTP.
- `cmd/manager/` : composition des dépendances et cycle de vie du processus.

Les collecteurs ne sont jamais appelés directement depuis un handler. Un planificateur détenu par le Manager lance des cycles périodiques et s'arrête avec son contexte avant la fermeture des ressources.

## 4. Collecte système

Le collecteur système lit localement les compteurs CPU, charge, mémoire, swap, systèmes de fichiers, interfaces, uptime et informations d'OS accessibles. Les différences entre deux échantillons de compteurs permettent de calculer les taux et débits. `/proc`, `/sys` et les bibliothèques Go spécialisées sont privilégiés aux commandes shell répétées.

Les températures sont opportunistes : une source absente ou inaccessible produit une donnée indisponible, jamais une valeur estimée présentée comme réelle. Le collecteur expose ses erreurs et l'heure de dernière réussite.

## 5. Collecte Docker

Un service de découverte interroge périodiquement la liste complète des conteneurs, arrêtés compris, et compare les identifiants/états avec l'état connu. Les nouveaux objets, suppressions, redémarrages et changements d'état sont persistés comme événements. Les statistiques par conteneur proviennent de Docker Engine API (`stats` non-streaming), indépendamment des pages affichées.

Le port applicatif expose seulement les besoins nécessaires, notamment `ListContainers` et `ContainerStats`. L'adaptateur encode les appels API Docker et traduit les réponses vers des types métier ; aucun type du SDK Docker ne traverse les ports/usecases. Les changements d'état sont identifiés par réconciliation périodique, pas par un flux d'événements.

Un proxy local en liste blanche est la frontière de sécurité. Il n'expose que les appels GET de liste, inspection et statistiques ; il refuse toute autre route, méthode, tunnel ou changement de daemon. Le processus Manager n'accède pas directement au socket rootful. Voir [SECURITY.md](./SECURITY.md).

En cas d'indisponibilité du démon, conserver le dernier inventaire connu comme périmé, marquer la collecte en erreur, puis réessayer avec délai borné. Ne pas supprimer l'état connu parce qu'une requête temporaire a échoué.

## 6. Planification, persistance et ressources

- Les cycles système, découverte Docker, stats Docker et rétention ont des fréquences/configurations indépendantes et bornées.
- La collecte démarre au lancement du Manager, non à la première requête.
- Une panne ponctuelle d'un sous-système n'empêche pas l'autre collecteur de fonctionner.
- Les insertions de métriques sont groupées ; les requêtes historiques ont des bornes temporelles et un plafond de résultats.
- La purge est paginée/par lots afin de limiter les verrous et le travail PostgreSQL.
- Toute goroutine a un propriétaire, une stratégie d'arrêt et des timeouts.

Les périodes et formules sont définies dans [METRICS_COLLECTION.md](./METRICS_COLLECTION.md). Le modèle de persistance est décrit dans [DATA_MODEL.md](./DATA_MODEL.md).

## 7. API et interface

Le Frontend consomme exclusivement l'API REST de même origine. REST est aussi la source de vérité pour le rafraîchissement périodique des métriques ; aucun WebSocket applicatif n'est requis. TanStack Query gère rafraîchissement, chargement, erreur et fraîcheur.

Les routes, DTO et limites sont décrits dans [API_CONTRACTS.md](./API_CONTRACTS.md). Le navigateur ne reçoit ni identifiants Docker ni accès à PostgreSQL.

## 8. Déploiement Debian

Le Manager est un service natif Debian 13, avec un utilisateur système dédié sans appartenance directe au groupe `docker`, un fichier de configuration protégé, un accès au proxy Docker local et une connexion PostgreSQL locale. Le Frontend compilé est servi par le Manager ou par un serveur local de même origine. Production ne requiert pas Docker Compose.

L'arborescence cible ne comprend qu'un module Backend Go et le Frontend :

```text
homelab-manager/
├── backend/
│   ├── cmd/manager/
│   ├── internal/{domain,usecase,ports,adapters,delivery/http}/
│   ├── ent/schema/
│   ├── migrations/
│   └── go.mod
├── frontend/
├── deployments/systemd/       # Manager et proxy Docker restreint
├── docs/
└── scripts/
```

Le Manager sert le contenu compilé de `frontend/dist` depuis la même origine. Le proxy est socket-activé par systemd ; son unité est séparée du Manager. Voir le [README](../README.md) pour la préparation des comptes, la configuration et l'installation.
