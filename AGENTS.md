# Instructions pour les agents — Homelab Manager

Fichier de référence unique pour tout assistant de code (Claude Code, Cursor, Codex, Copilot…). `CLAUDE.md`, `.cursor/rules/` et `.github/copilot-instructions.md` ne font que renvoyer ici : modifier ce fichier, pas leurs copies.

## Référence obligatoire

Lire `README.md` et tous les fichiers de `docs/` avant toute modification structurante. Ils sont la source de vérité (v2.1) :

| Document | Rôle |
|---|---|
| `docs/PRODUCT_SPEC.md` | Périmètre, décisions produit, limites connues |
| `docs/API_CONTRACTS.md` | **Contrat exact** de l'API : à modifier dans le même commit que le code |
| `docs/ARCHITECTURE.md`, `docs/DATA_MODEL.md`, `docs/METRICS_COLLECTION.md` | Conception, schéma, formules |
| `docs/SECURITY.md` | Exigences de sécurité |
| `docs/TESTING.md` | Stratégie et couverture des tests |
| `docs/CODE_RULES.md` | **Règles d'écriture du code**, Go et TypeScript, Git |
| `docs/SPEC_ADMINISTRATION.md` | Modules d'administration v3 (spécifiés, non implémentés) |

## Périmètre

- Homelab Manager supervise une seule machine Linux, celle du Manager. Pas d'hôte distant, d'Agent de collecte, de multi-agent ni de synchronisation entre machines.
- **Interdits absolus** : aucun terminal, shell ni SSH dans l'application (ni console web, ni `exec` de conteneur) ; les utilisateurs sont uniquement ceux de Homelab Manager (aucun compte Linux, PAM, sudo ni SSH de la machine) ; aucune exécution de code fourni par l'utilisateur.
- Les applications PaaS / GitOps restent gérées par Coolify : ne pas les implémenter.
- Ne pas inventer de fonctionnalité hors spécification. Une maquette de `maquette/` n'est pas une spécification.

## Stack validée

- Backend Go, Chi v5, Ent, SQLite fichier unique (`modernc.org/sqlite`, sans CGO), Viper, zap.
- Collecte locale par `/proc`, `/sys` et bibliothèques Go ; Docker Engine API par socket Unix derrière un proxy en liste blanche, en lecture seule.
- Frontend React, TypeScript strict, Vite, React Router, TanStack Query, Tailwind CSS, React Hook Form + Zod, Apache ECharts ; formatage Prettier.
- Tests : `testing`, `httptest`, SQLite réel sur fichier temporaire ; Vitest + React Testing Library.
- Déploiement natif Debian 13 / systemd, sans serveur de base de données ; empreinte mémoire minimale.
- Ne pas remplacer Chi par Gin, Ent par GORM, ni réintroduire un serveur de base de données sans nouvelle exigence explicite.

## Contraintes d'architecture

- Hexagonale : domain → usecase → ports ← adapters. Le domaine ne dépend ni de Chi, Ent, SQLite, Docker ni de React.
- Chi uniquement dans `delivery/http` : DTO, validation, appel des usecases. Ent uniquement dans les adaptateurs ; jamais d'entité Ent en DTO.
- Collecte indépendante des requêtes HTTP et des utilisateurs connectés.
- Pas de logique métier dans les handlers. Côté React, **aucun code métier dans les pages et composants** : requêtes, mutations, formulaires, calculs, filtres et décisions vont dans des hooks `use…` (un par page, `src/features/<domaine>/use<Page>.ts`) et des fonctions pures de `src/lib/` ; les composants ne font qu'afficher le modèle de vue (règle détaillée et contrôle ESLint : `docs/CODE_RULES.md` § 4.3).
- Pas de dépendance cloud pour le fonctionnement normal.
- Pas d'API Docker mutatrice. Toute action privilégiée future suit le modèle de `docs/SPEC_ADMINISTRATION.md` § 2 (D-Bus, polkit en liste blanche, unités systemd dédiées) ; le Manager ne tourne jamais en root.

## Sécurité et qualité

- Validation serveur systématique ; schémas Zod côté interface (jamais une protection à eux seuls).
- Authentification locale, mots de passe Argon2id, sessions sécurisées, contrôle d'origine (CSRF).
- Ne jamais exposer au navigateur le socket Docker, le proxy, les variables d'environnement ni un secret.
- Toute goroutine a un propriétaire, une stratégie d'arrêt et des limites de temps et de ressources.
- Toute erreur est traitée explicitement ; pas de `panic` pour un cas attendu.
- Toute dépendance nouvelle est justifiée (usage, impact) et figée à une version publiée depuis au moins deux semaines.

## Avant de livrer

```bash
scripts/verify.sh
```

Il vérifie `gofmt`, `go vet`, `go test` (avec et sans `-race`), Prettier, ESLint, TypeScript, Vitest et le build. Il doit passer.

## Règles de modification

1. Examiner le code et les documents avant de modifier.
2. Pour un changement transverse, donner un plan court d'abord.
3. Préserver les contrats ou documenter la migration.
4. Ajouter les tests avec le code ; un bug corrigé a un test qui échouait avant.
5. Mettre à jour les documents concernés dans le même commit.
6. Suivre `docs/CODE_RULES.md` : code et commits en anglais, documentation et interface en français, Conventional Commits.
7. Ne jamais versionner `.env`, bases SQLite, binaires ni secrets.
