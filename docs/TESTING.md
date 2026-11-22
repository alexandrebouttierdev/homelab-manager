# Stratégie de tests — Homelab Manager

Version 2.1 — 22 novembre 2026

## Backend Go

### Collecte système
- Tester le parsing avec fixtures `/proc` et `/sys`, sans dépendre des ressources de la machine de CI.
- Tester compteurs absents, données malformées, permissions insuffisantes, capteurs indisponibles et annulation de contexte.
- Tester les calculs de deltas CPU, mémoire, stockage et débit réseau, y compris compteurs réinitialisés et intervalles nuls.
- Ne jamais exiger une température matérielle réelle dans les tests.

### Docker
- Définir un port d'adaptateur simulable ; les tests unitaires n'exigent pas de daemon Docker.
- Tester conteneurs arrêtés, nouveaux/supprimés/redémarrés, erreurs temporaires, cgroups v1/v2 et champs non disponibles.
- Tester formules CPU/mémoire et agrégation réseau/bloc avec fixtures de réponses Engine API.
- Tester que routes/méthodes non autorisées sont refusées par le proxy et que données sensibles (comme Env) ne sont pas exposées.
- Tests d'intégration optionnels contre un daemon local dédié, sans modifier des conteneurs de l'hôte de développeur.

### Usecases, persistance et HTTP
- Tester planification, indépendance des collecteurs, transitions et rétention avec horloge contrôlée et ports simulés.
- Tests SQLite réels (fichier temporaire, mêmes pragmas que la production) pour migrations, transactions, purge, requêtes temporelles et accès concurrents.
- `httptest` pour routes Chi, DTO, auth, limites, erreur, bornes temporelles et contrôle d'accès.
- Tester annulation et arrêt ordonné de chaque goroutine de collecte.

Commandes du module cible :
```bash
gofmt -w .
go vet ./...
go test ./...
go test -race ./...
```

Les tests d'intégration SQLite (`internal/adapters/sqlite`) s'exécutent avec `go test ./...` sans configuration : chaque test ouvre une base neuve dans `t.TempDir()` via le même `Open` que le Manager. Pour les lancer seuls :

```bash
go test ./internal/adapters/sqlite -count=1 -v
```

## Frontend React

- Vitest + React Testing Library pour dashboard, navigation, détails machine/conteneur et états loading/empty/error/stale.
- Tester graphiques avec données/vides/erreurs, fenêtres temporelles, labels et fraîcheur.
- Tester schémas Zod, formulaires, authentification, client REST et paramètres.
- Vérifier interactions clavier, labels et responsive.
- Aucun test ne doit afficher des données simulées comme des données réelles.

Commandes :
```bash
npm run format:check
npm run lint
npm run typecheck
npm run test
npm run build
```

## Parcours transverses

- Démarrage sans Docker puis récupération après indisponibilité du daemon.
- Détection à chaud de création, arrêt, redémarrage et suppression d'un conteneur.
- Collecte système poursuivie si Docker est indisponible, et inversement.
- Historique, bornes temporelles et rétention.
- Authentification, contrôle d'origine, sessions révoquées et accès proxy strictement local.
- Démarrage/arrêt natif Debian 13 via systemd et vérification des permissions des sockets.

### Couverture actuelle

Les sections précédentes fixent les exigences. Aujourd'hui couverts automatiquement : formules CPU (deltas `/proc/stat`), débits réseau/disque et remises à zéro, formules CPU/mémoire Docker, découverte Docker avec conteneur disparu (API simulée), liste blanche du proxy, usecases (indépendance des collecteurs, arrêt, état `stale`, horodatage de `restart.count`, purge complète d'un arriéré, rétention choisie dans l'interface et purge immédiate, anti-énumération à la connexion), routes HTTP (santé, SPA, 404/405 JSON, contrôle d'origine, validation des paramètres), intégration SQLite (voir plus bas), dont l'agrégation (moyenne, min, max, nombre par intervalle), le filtre par label, le refus `TOO_MANY_POINTS` et le stockage de la rétention avec son audit, configuration, client REST, schémas Zod (connexion, mot de passe, rétention) et estimation de taille du Frontend, pages de connexion et dashboard.

Non couverts automatiquement, à vérifier manuellement avant une version : parcours transverses ci-dessus, démarrage systemd réel et permissions des sockets, accessibilité clavier et affichage mobile.

Les tests doivent valider la cible mono-machine ; aucun test Agent distant, enrôlement ou protocole Manager-Agent ne fait partie de la spécification.

L'intégration SQLite vérifie la réouverture sans réappliquer les migrations, les pragmas effectifs (WAL, clés étrangères, `busy_timeout`), le mode `0600` du fichier, l'authentification/persistance des sessions, l'hôte, les métriques, la réconciliation des conteneurs, les événements, la purge, le dernier cycle complet (`LatestMetrics`), l'ordre chronologique des plages quel que soit le fuseau des dates reçues, et les écritures/lectures concurrentes. Les tests unitaires vérifient également les formules Docker, les deltas système, la liste blanche du proxy et les routes HTTP de base.
