# Stratégie de tests — Homelab Manager

Version 2.0 — 21 novembre 2026

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
- Tests PostgreSQL réels pour migrations, index, transactions, purge et requêtes temporelles ; pas de SQLite substitut.
- `httptest` pour routes Chi, DTO, auth, limites, erreur, bornes temporelles et contrôle d'accès.
- Tester annulation et arrêt ordonné de chaque goroutine de collecte.

Commandes du module cible :
```bash
gofmt -w .
go vet ./...
go test ./...
go test -race ./...
```

## Frontend React

- Vitest + React Testing Library pour dashboard, navigation, détails machine/conteneur et états loading/empty/error/stale.
- Tester graphiques avec données/vides/erreurs, fenêtres temporelles, labels et fraîcheur.
- Tester schémas Zod, formulaires, authentification, client REST et paramètres.
- Vérifier interactions clavier, labels et responsive.
- Aucun test ne doit afficher des données simulées comme des données réelles.

Commandes :
```bash
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

Les tests doivent valider la cible mono-machine ; aucun test Agent distant, enrôlement ou protocole Manager-Agent ne fait partie de la spécification v2.0.

L'intégration PostgreSQL n'est pas activée par défaut : configurer une instance de test dédiée avant d'ajouter et d'exécuter ces tests. Les tests unitaires actuels vérifient les formules Docker, les deltas système, la liste blanche du proxy et les routes HTTP de base.
