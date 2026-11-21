# Instructions Claude Code — Homelab Manager

## Référence obligatoire
Lire `README.md` et tous les fichiers `docs/` avant toute modification structurante. Ces documents sont la source de vérité du projet.

## Stack validée
- Backend : Go ; routeur HTTP **Chi v5** (`github.com/go-chi/chi/v5`).
- ORM : Ent (`entgo.io/ent`).
- Base : PostgreSQL dédié.
- WebSocket : `github.com/coder/websocket`.
- Configuration : Viper.
- Logs structurés : Zap.
- Frontend : React, TypeScript, Vite.
- Formulaires : React Hook Form + Zod.
- Graphiques : Apache ECharts.
- Tests frontend : Vitest + React Testing Library.
- Tests backend : `testing` standard Go, `httptest`, tests d'intégration PostgreSQL.

## Contraintes d'architecture
- Architecture hexagonale / ports-adapters : Domain → Usecase → ports ; les adaptateurs dépendent des ports, jamais l'inverse.
- Le domaine ne dépend ni de Chi, ni d'Ent, ni de PostgreSQL, ni de React.
- Chi est uniquement dans la couche Delivery/HTTP. Les handlers convertissent HTTP vers des DTO et appellent les usecases.
- Ent est confiné aux adaptateurs de persistance. Ne jamais exposer les entités Ent comme DTO API.
- Séparer clairement Manager et Agent. L'Agent ne doit pas embarquer de logique métier du Manager.
- Pas de logique métier dans les handlers HTTP ou les composants React.
- Pas de dépendance à un service cloud pour le fonctionnement normal.
- Pas d'exécution de commandes arbitraires envoyées par l'interface ou le Manager.

## Qualité obligatoire
- DTO d'entrée/sortie explicites et validation côté serveur.
- Schémas Zod côté frontend ; ne jamais considérer la validation frontend comme une protection suffisante.
- Tests unitaires pour chaque usecase et logique métier.
- Tests des handlers HTTP avec `httptest`.
- Tests d'intégration des repositories contre PostgreSQL réel (par exemple via environnement de test dédié).
- Tests React des composants, formulaires, états de chargement/erreur et interactions.
- `go test ./...`, `go test -race ./...`, `go vet ./...`, `gofmt`.
- `npm run lint`, `npm run typecheck`, `npm run test`, `npm run build`.
- Toute goroutine doit avoir un propriétaire, une stratégie d'arrêt et une limite de durée ou de ressources.
- Toute erreur doit être traitée ; pas de panic pour un flux d'erreur attendu.
- Ne pas introduire de dépendance sans expliquer son utilité et son impact.

## Règles de modification
1. Examiner le code existant avant de modifier.
2. Proposer un plan court pour les changements transverses.
3. Préserver les contrats existants ou documenter une migration.
4. Ajouter les tests dans le même changement que le code.
5. Mettre à jour les documents concernés.
6. Ne pas remplacer Chi par Gin ou un autre framework.
7. Ne pas inventer des fonctionnalités absentes des spécifications.
