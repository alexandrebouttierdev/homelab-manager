# Règles de code — Homelab Manager

Version 2.1 — 22 novembre 2026

Ces règles s'appliquent à toute contribution, humaine ou assistée par un agent (voir [AGENTS.md](../AGENTS.md)). Une règle « doit » est vérifiée en revue ; les outils automatiques sont indiqués quand ils existent. Les exigences d'architecture et de sécurité sont dans ARCHITECTURE.md et SECURITY.md ; ce document fixe la façon d'écrire le code.

## 1. Langues

| Élément | Langue |
|---|---|
| Code, identifiants, commentaires, messages de log, messages d'erreur de l'API, messages de commit | Anglais |
| Documentation (`docs/`, README), textes affichés dans l'interface, messages d'erreur montrés à l'utilisateur | Français |

L'interface ne montre jamais un message d'erreur brut de l'API : elle traduit le `code` (`frontend/src/lib/api/client.ts`, `errorMessages`).

## 2. Vérifications obligatoires

`scripts/verify.sh` doit passer avant tout commit. Il enchaîne :

| Côté | Contrôle | Commande |
|---|---|---|
| Go | Formatage | `gofmt -l backend` (doit être vide) |
| Go | Tests et analyse | `go test ./...`, `go test -race ./...`, `go vet ./...` |
| Frontend | Formatage | `npm run format:check` (Prettier) |
| Frontend | Lint, types, tests, build | `npm run lint`, `npm run typecheck`, `npm run test`, `npm run build` |
| Release | Calcul de version et notes | `scripts/release/release_test.sh` (dépôt Git jetable) |

Corriger le formatage : `gofmt -w backend` et `npm --prefix frontend run format`. Un avertissement du linter est traité comme une erreur.

## 3. Backend Go

### 3.1 Structure

- Couches hexagonales : `domain` → `usecase` → `ports` ← `adapters` ; `delivery/http` appelle les usecases ; `cmd/manager` assemble. Un package n'importe jamais une couche plus externe que la sienne.
- `domain` : types et erreurs sentinelles (`var ErrX = errors.New(...)`), sans dépendance à Chi, Ent, SQLite, Docker ni zap.
- `ports` : interfaces minimales, documentées quand la sémantique n'est pas évidente (ordre, bornes, cas vide).
- `delivery/http` : DTO avec tags JSON, validation de **tous** les paramètres, traduction des erreurs du domaine en codes HTTP du contrat. Aucune règle métier.
- `adapters/sqlite` : seul endroit où Ent et SQL apparaissent. SQL brut autorisé uniquement pour ce qu'Ent n'exprime pas (agrégation), toujours avec des paramètres liés (`?`), jamais par concaténation de valeurs.
- Les entités Ent ne sortent jamais des adaptateurs.

### 3.2 Style

- `gofmt` ; noms Go idiomatiques (`ID`, `HTTP`, `URL` en majuscules) ; pas de bégaiement (`sqlite.Open`, pas `sqlite.OpenSQLite`).
- `context.Context` en premier paramètre de toute fonction qui fait une E/S.
- Constantes nommées pour toute borne, durée ou limite (`maxHistoryLimit`, `currentWindow`), avec un commentaire si la valeur n'est pas évidente.
- Commentaires : ils expliquent le *pourquoi* ou une contrainte non visible, pas ce que le code dit déjà. Tout type ou fonction exporté porte un commentaire s'il a une sémantique non triviale.
- Pas de code mort : une fonction, une méthode de port ou une colonne inutilisée est supprimée.

### 3.3 Erreurs

- Toute erreur est traitée ou renvoyée ; `_ =` seulement pour un nettoyage en chemin d'erreur (`tx.Rollback`) ou une écriture de réponse HTTP.
- Contexte ajouté avec `fmt.Errorf("action: %w", err)` ; comparaison par `errors.Is` / `errors.As`, jamais par chaîne ni par `==`.
- Pas de `panic` pour un cas prévisible ; pas de `log.Fatal` hors de `main`.
- Une erreur renvoyée au client ne contient ni chemin, ni secret, ni détail interne (voir API_CONTRACTS.md § 8).

### 3.4 Concurrence et temps

- Toute goroutine a un propriétaire qui l'attend (`sync.WaitGroup`), s'arrête par annulation de contexte et a un délai maximal par cycle.
- Pas de file d'attente non bornée ; canaux avec capacité explicite.
- Horodatages en UTC (`.UTC()`) ; l'horloge est injectable (`now func() time.Time`) dans tout code dont le comportement dépend du temps, pour le tester.
- SQLite : une seule connexion ; une transaction n'utilise que son `tx`.

### 3.5 Journaux

- zap, champs structurés (`zap.String("container_id", id)`), message en anglais, sans donnée sensible (mot de passe, cookie, jeton, secret, variable d'environnement).

### 3.6 Tests

- Fichier `_test.go` à côté du code ; nom `TestComportementAttendu` (ex. `TestPurgeDrainsTheWholeBacklog`).
- Tests en tableau pour les validations ; faux objets écrits à la main (pas de framework de mock) ; `httptest` pour HTTP ; base SQLite réelle dans `t.TempDir()` pour l'adaptateur, jamais un mock du stockage.
- Chaque correction de bug ajoute un test qui échoue sans la correction.
- Pas de dépendance aux ressources de la machine (températures, Docker réel) dans les tests unitaires.

## 4. Frontend React / TypeScript

### 4.1 Formatage et analyse

- **Prettier** (`frontend/.prettierrc.json`) : sans point-virgule, guillemets simples, virgule finale, 100 colonnes, fins de ligne LF. Ne pas reformater à la main : lancer `npm run format`.
- ESLint (`eslint.config.js`) : règles TypeScript recommandées, règles des hooks React. Aucun `eslint-disable` sans commentaire justifiant l'exception.
- TypeScript strict ; `any` interdit (utiliser `unknown` puis un rétrécissement) ; pas d'assertion `as` pour contourner une erreur de type, sauf sur une donnée JSON validée.

### 4.2 Organisation

| Dossier | Contenu |
|---|---|
| `src/features/<domaine>/` | Pages et composants propres à un domaine (`containers`, `settings`…) et leurs hooks `use<Page>.ts` |
| `src/hooks/` | Hooks partagés entre domaines (`useSession`, `useHost`, `loadState`…) |
| `src/components/ui/` | Composants génériques sans connaissance métier (Button, Card, Badge…) |
| `src/components/charts/` | Enveloppes ECharts ; les pages n'importent pas `echarts-for-react` directement |
| `src/lib/api/` | Seul accès au réseau, via `requestJSON` ; un fichier par domaine d'API ; types des DTO |
| `src/lib/` | Logique pure testable (formatage, calculs de séries, thème) |
| `src/schemas/` | Schémas Zod des formulaires, avec leurs tests |

- Un composant par fichier exporté ; nom en PascalCase identique au fichier.

### 4.3 Aucun code métier dans les pages et composants

Les pages et composants (`.tsx` de `src/app`, `src/components`, `src/features`) **affichent uniquement**. Toute la logique est dans des hooks `use…` :

| Autorisé dans un composant | Dans un hook `use…` (fichier `.ts`) | Dans `src/lib/` (fonctions pures, testées) |
|---|---|---|
| Appeler un hook et déstructurer son modèle de vue | `useQuery`, `useMutation`, invalidations, appels à `src/lib/api/` | Calculs, filtres, tris, regroupements, résumés |
| JSX, conditions d'affichage sur des valeurs déjà calculées | Formulaires (`useForm`, `zodResolver`, `useWatch`) et soumission | Seuils et décisions (`usageTone`, `matchesStateFilter`…) |
| Formatage d'affichage (`formatBytes`, `formatPercent`, `Math.round` d'un nombre affiché) | Dérivations (`useMemo`), effets (`useEffect`), navigation après action | Traductions de valeurs (`collectorStatusLabel`…) |
| État purement visuel (`useState` d'un menu ouvert, d'un mot de passe affiché) | État des filtres, de la recherche et de la période | |
| `.map()` qui transforme une liste déjà prête en éléments JSX | Construction des options ECharts | |

- Chaque page a son hook `src/features/<domaine>/use<Page>.ts` qui renvoie un **modèle de vue** : valeurs prêtes à afficher, actions (`refresh`, `onSubmit`, `retry`…) et états de chargement via `loadState` (`isLoading`, `isError`, `error`, `refetch`), jamais l'objet de requête TanStack brut ni `.data`.
- Un hook ne renvoie que ce que la page affiche ; les intermédiaires restent internes.
- Contrôle automatique : ESLint interdit aux `.tsx` de `src/app`, `src/components` et `src/features` d'importer `@tanstack/react-query`, `src/lib/api/*` (hors imports de types) et `useMemo`, `useEffect`, `useLayoutEffect`, `useReducer`. Un calcul écrit directement dans le JSX (`.filter`, `.reduce`, `.sort`, `new Set`…) n'est pas détecté par l'outil : il est refusé en revue.

### 4.4 Données et formulaires

- TanStack Query pour toute donnée serveur ; clés de requête hiérarchiques et stables (`['host', 'metrics', name, label, range]`) ; `refetchInterval` dérivé de `useRefreshInterval()`.
- Formulaires : React Hook Form + `zodResolver` ; `useWatch` plutôt que `watch` ; les bornes du schéma Zod reprennent celles du contrat d'API. La validation de l'interface n'est jamais une protection.
- Aucune donnée simulée présentée comme réelle ; une donnée absente s'affiche comme indisponible, jamais comme zéro.
- Version de l'application : `src/lib/version.ts` uniquement.

### 4.5 Interface

- Couleurs et espacements par les jetons Tailwind du thème (`text-text-primary`, `bg-card`, `border-line`…), jamais de couleur codée en dur.
- Accessibilité : chaque champ a un `label` associé ; messages d'erreur en `role="alert"`, confirmations en `role="status"` ; boutons d'icône avec `aria-label`.
- Thèmes clair et sombre et affichage mobile vérifiés pour toute nouvelle page.

### 4.6 Tests

- Vitest + React Testing Library ; tester le comportement visible (textes, rôles), pas l'implémentation.
- Tout schéma Zod, toute fonction de `src/lib/`, tout utilitaire de `src/hooks/` et tout client d'API ont des tests. La logique d'un hook de page se teste par les fonctions pures qu'il appelle ; un hook à comportement propre (enchaînement d'actions, navigation) se teste avec `renderHook`.

## 5. API et documentation

- `docs/API_CONTRACTS.md` est le contrat exact : toute route, paramètre, borne, DTO ou code d'erreur ajouté ou modifié l'est dans le même commit que le code.
- Un changement de comportement met à jour le document qui le décrit (METRICS_COLLECTION.md, DATA_MODEL.md, SECURITY.md…) dans le même commit.
- Nouvelle table ou colonne : nouvelle migration numérotée (`00N_description.sql`), jamais de modification d'une migration livrée ; schéma Ent régénéré depuis `backend/` avec `GOTOOLCHAIN=go1.26.10 go run entgo.io/ent/cmd/ent@v0.14.6 generate ./ent/schema`. Fixer `GOTOOLCHAIN` est nécessaire : `go run …@v0.14.6` est lancé hors du module et prendrait sinon le Go de base de la machine, trop ancien pour le `go.mod` du projet ; en cas d'échec, restaurer `ent/runtime.go` (`git checkout`), que le générateur laisse modifié.

## 6. Dépendances

- Toute nouvelle dépendance (Go ou npm) est justifiée dans la description du commit : usage, alternatives écartées, impact (taille, maintenance, sécurité).
- Version figée ; version publiée depuis au moins deux semaines.
- Préférer la bibliothèque standard et le code interne court et testé à une dépendance pour quelques lignes.

## 7. Git

- Branche de travail `dev` ; `main` reçoit des fusions de `dev`.
- Chaque push sur `main` publie une release (`.github/workflows/release.yml`) : vérification complète, archives Linux `amd64` et `arm64`, `SHA256SUMS`, tag `vMAJEUR.MINEUR.CORRECTIF`. La version se déduit des Conventional Commits depuis le dernier tag (`scripts/release/next-version.sh`) : `type!:` ou `BREAKING CHANGE:` → majeure, `feat` → mineure, le reste → correctif ; la première est `2.3.0`. Le type d'un commit compte donc : un `feat` qui n'ajoute rien de visible fausse la version.
- La version est injectée à la construction (`-X main.version`, `VITE_APP_VERSION`) ; ne jamais l'écrire en dur dans le code.
- Messages au format Conventional Commits, en anglais : `type(scope): résumé à l'impératif`, avec `type` ∈ `feat`, `fix`, `refactor`, `perf`, `test`, `docs`, `chore`, `build` ; corps expliquant le pourquoi.
- Un commit = un changement cohérent, avec ses tests et sa documentation, qui passe `scripts/verify.sh`.
- Ne jamais versionner : `.env`, bases SQLite (`*.db`, `-wal`, `-shm`), binaires, `dist/`, secrets.
