# Instructions Claude Code — Homelab Manager

## Référence obligatoire
Lire `README.md` et tous les fichiers `docs/` avant toute modification structurante. Ces documents v2.1 sont la source de vérité ; `docs/API_CONTRACTS.md` est le contrat exact de l'API.

## Décision de périmètre
Homelab Manager supervise définitivement une seule machine Linux : celle du Manager. La collecte système et Docker est locale et intégrée au Backend. Ne pas introduire de support d'hôtes distants, d'Agent de collecte, de multi-agent ou de synchronisation entre machines.

## Stack validée
- Backend Go, routeur HTTP Chi v5 (`github.com/go-chi/chi/v5`).
- ORM Ent (`entgo.io/ent`), SQLite local en fichier unique (driver Go pur `modernc.org/sqlite`, sans CGO).
- Configuration Viper ; logs structurés Zap.
- Collecte système Linux via interfaces locales (`/proc`, `/sys`) et bibliothèques Go adaptées.
- Docker Engine API locale par socket Unix, derrière un adaptateur d'infrastructure restreint.
- Frontend React, TypeScript strict, Vite, React Router, TanStack Query, Tailwind CSS.
- Formulaires React Hook Form + Zod ; graphiques Apache ECharts.
- Tests Backend `testing`, `httptest`, intégration SQLite sur fichier temporaire ; Frontend Vitest + React Testing Library.
- Déploiement natif Debian 13/systemd, sans service de base de données séparé ; l'empreinte mémoire reste un objectif (homelab).

## Contraintes d'architecture
- Architecture hexagonale : Domain → Usecase → ports ; adaptateurs dépendant des ports, jamais l'inverse.
- Le domaine ne dépend ni de Chi, Ent, SQLite, Docker ni de React.
- Chi uniquement dans Delivery/HTTP ; handlers = DTO/validation/appel des usecases.
- Ent confiné aux adaptateurs de persistance ; ne jamais exposer ses entités comme DTO.
- Collecte indépendante des handlers et des utilisateurs connectés.
- Docker derrière un port et un proxy local en liste blanche ; le Manager n'est pas ajouté directement au groupe `docker` dans le modèle cible.
- Pas de logique métier dans les handlers ou composants React.
- Pas de dépendance cloud pour le fonctionnement normal.
- Aucune exécution de commandes arbitraires ; pas d'API Docker mutatrice ou d'administration de conteneurs sans nouvelle exigence explicite.

## Sécurité et qualité obligatoires
- DTO explicites, validation serveur systématique, schémas Zod côté UI ; la validation UI n'est pas une protection.
- Authentification locale, mots de passe hachés, sessions sécurisées et protections CSRF.
- Ne jamais exposer le socket Docker, proxy, variables d'environnement ou secrets au navigateur.
- Tests unitaires de logique métier et collecteurs avec fixtures/mocks, tests `httptest` et SQLite réel (pas de mock du stockage pour les tests d'adaptateur).
- `go test ./...`, `go test -race ./...`, `go vet ./...`, `gofmt`.
- `npm run lint`, `npm run typecheck`, `npm run test`, `npm run build`.
- Toute goroutine a un propriétaire, une stratégie d'arrêt et des limites de ressources/temps.
- Toute erreur est traitée explicitement ; pas de panic pour un flux attendu.
- Ne pas ajouter de dépendance sans expliquer son utilité et son impact.

## Règles de modification
1. Examiner le code et les documents avant modification.
2. Pour un changement transverse, donner un plan court.
3. Préserver les contrats ou documenter une migration.
4. Ajouter les tests avec le code.
5. Mettre à jour les documents concernés.
6. Ne pas remplacer Chi par Gin ou Ent par GORM, ni réintroduire un serveur de base de données (PostgreSQL, MySQL…) sans nouvelle exigence explicite.
7. Ne pas inventer des fonctionnalités hors spécification.
