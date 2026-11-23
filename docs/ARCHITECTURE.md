# Architecture technique — Homelab Manager

Version 2.1 — 22 novembre 2026

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
  |-- Adaptateur Ent / SQLite
          |
          +---- Unix socket local ---- Docker Engine
          |
          +---- Fichier SQLite local (/var/lib/homelab-manager)
```

Le périmètre est un hôte Linux unique : celui qui exécute le Manager. Aucun collecteur, transport ou modèle n'est conçu pour un hôte distant.

## 2. Stack et responsabilités

| Besoin | Technologie / règle |
|---|---|
| Langage Backend | Go |
| API HTTP | Chi v5 et `net/http` |
| ORM / persistance | Ent / SQLite embarqué (`modernc.org/sqlite`, Go pur) |
| Configuration | Viper |
| Logs | Zap |
| Collecte système | Interfaces Linux locales (`/proc`, `/sys`) et bibliothèques Go adaptées |
| Collecte Docker | Docker Engine API sur socket Unix local, derrière un port d'infrastructure |
| Frontend | React, TypeScript strict, Vite, React Router |
| Styles et composants UI | Tailwind CSS v4, composants React internes |
| Cache serveur UI | TanStack Query |
| Formulaires / validation UI | React Hook Form / Zod |
| Graphiques | Apache ECharts |
| Tests | `testing`, `httptest`, Vitest et React Testing Library |

Chi reste exclusivement dans Delivery/HTTP. Ent reste dans les adaptateurs de persistance. Le domaine ne dépend ni de Chi, ni d'Ent, ni de Docker, ni de SQLite, ni de React.

## 3. Architecture hexagonale

- `domain/` : objets et règles métier pour l'hôte, les conteneurs, les mesures et événements.
- `usecase/` : orchestration des lectures, collectes, historiques et paramètres.
- `ports/` : interfaces minimales pour les collecteurs, repositories et horloge si nécessaire.
- `adapters/` : Linux, API Docker, SQLite/Ent, système de fichiers, `systemd-logind` et gestionnaire systemd (D-Bus, en lecture pour ce dernier), journal système (`journalctl`), coffre de secrets et QR code.
- `delivery/http/` : handlers Chi, DTO, middleware et validation HTTP.
- `cmd/manager/` : composition des dépendances et cycle de vie du processus.

Les collecteurs ne sont jamais appelés directement depuis un handler. Le journal système fait exception à la collecte périodique : il est lu à la demande, par `journalctl`, sans copie en base (le journal reste la source) ; la synthèse sur 24 h et la liste des démarrages sont gardées une minute en mémoire. Un planificateur détenu par le Manager lance des cycles périodiques et s'arrête avec son contexte avant la fermeture des ressources.

## 4. Collecte système

Le collecteur système lit localement les compteurs CPU, charge, mémoire, swap, systèmes de fichiers, interfaces, uptime et informations d'OS accessibles. Les différences entre deux échantillons de compteurs permettent de calculer les taux et débits. `/proc`, `/sys` et les bibliothèques Go spécialisées sont privilégiés aux commandes shell répétées.

Les températures sont opportunistes : une source absente ou inaccessible produit une donnée indisponible, jamais une valeur estimée présentée comme réelle. Le collecteur expose ses erreurs et l'heure de dernière réussite.

## 5. Collecte Docker

Un service de découverte interroge périodiquement la liste complète des conteneurs, arrêtés compris, et compare les identifiants/états avec l'état connu. Les nouveaux objets, suppressions, redémarrages et changements d'état sont persistés comme événements. Les statistiques par conteneur proviennent de Docker Engine API (`stats` non-streaming), indépendamment des pages affichées.

Le port applicatif expose seulement les besoins nécessaires, notamment `ListContainers` et `ContainerStats`. L'adaptateur encode les appels API Docker et traduit les réponses vers des types métier ; aucun type du SDK Docker ne traverse les ports/usecases. Les changements d'état sont identifiés par réconciliation périodique, pas par un flux d'événements.

Un proxy local en liste blanche est la frontière de sécurité. Il n'expose que les appels GET de liste, inspection et statistiques ; il refuse toute autre route, méthode, tunnel ou changement de daemon. Le processus Manager n'accède pas directement au socket rootful. Voir [SECURITY.md](./SECURITY.md).

En cas d'indisponibilité du démon, conserver le dernier inventaire connu comme périmé, marquer la collecte en erreur, puis réessayer avec délai borné. Ne pas supprimer l'état connu parce qu'une requête temporaire a échoué.

## 6. Planification, persistance et ressources

- Les cycles système, découverte Docker, stats Docker et rétention (horaire) ont des fréquences indépendantes et bornées, chacun dans une goroutine détenue par `MonitoringUsecase.Run`, arrêtée par annulation de contexte avant la fermeture de la base.
- La collecte démarre au lancement du Manager, non à la première requête.
- Une panne ponctuelle d'un sous-système n'empêche pas l'autre collecteur de fonctionner.
- Les insertions de métriques sont groupées ; les requêtes historiques ont des bornes temporelles et un plafond de résultats ; les vues courantes lisent le dernier cycle complet.
- Les services systemd (SPEC_ADMINISTRATION.md § 5.1) sont collectés toutes les 30 s par une goroutine détenue par `ServiceUsecase.Run` et arrêtée avec les autres avant la fermeture de la base : une connexion D-Bus par cycle, 25 s au plus. L'inventaire reste en mémoire avec son statut (`pending`, `ok`, `error`, `unavailable`) ; seul l'ensemble des unités en échec est enregistré (`failed_units`), avec les événements `service.failed` et `service.recovered`. Le détail d'une unité est lu à la demande, avec les 50 dernières entrées de son journal.
- L'état des collecteurs est tenu en mémoire par le usecase (`starting`, `ok`, `stale`, `unavailable`) et n'est pas persisté.
- La purge est paginée/par lots afin de limiter la durée de chaque écriture SQLite : la connexion unique est partagée avec les collecteurs et l'API.
- Toute goroutine a un propriétaire, une stratégie d'arrêt et des timeouts.

Les périodes et formules sont définies dans [METRICS_COLLECTION.md](./METRICS_COLLECTION.md). Le modèle de persistance est décrit dans [DATA_MODEL.md](./DATA_MODEL.md).

## 7. API et interface

Le Frontend consomme exclusivement l'API REST de même origine. Il suit la même séparation que le Backend : pages et composants pour l'affichage seul, hooks `use…` pour l'accès aux données et l'orchestration, fonctions pures de `src/lib/` pour les calculs (CODE_RULES.md § 4.3). REST est aussi la source de vérité pour le rafraîchissement périodique des métriques ; aucun WebSocket applicatif n'est requis. TanStack Query gère rafraîchissement, chargement, erreur et fraîcheur.

Les routes, DTO et limites sont décrits dans [API_CONTRACTS.md](./API_CONTRACTS.md). Le navigateur ne reçoit ni identifiants Docker ni accès à la base SQLite.

## 8. Déploiement Debian

Le Manager est un service natif Debian 13, avec un utilisateur système dédié sans appartenance directe au groupe `docker`, un fichier de configuration protégé, un accès au proxy Docker local et un répertoire d'état `/var/lib/homelab-manager` contenant la base SQLite. Le Frontend compilé est servi par le Manager ou par un serveur local de même origine. Production ne requiert ni Docker Compose ni serveur de base de données.

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
