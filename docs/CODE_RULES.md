# Règles de code — MonoOps

Version 2.2 — 24 novembre 2026 (audits de sécurité des dépendances, bonnes pratiques React, règles générales de sécurité)

Ces règles s'appliquent à toute contribution, humaine ou assistée par un agent (voir [AGENTS.md](../AGENTS.md)), à chaque création de fichier, modification, ajout ou mise à jour de dépendance, refactorisation, correction de bug et revue de code. Ce sont des exigences, pas des recommandations : une contribution qui ne les respecte pas n'est pas terminée. Une règle « doit » est vérifiée en revue ; les outils automatiques sont indiqués quand ils existent. Les exigences d'architecture et de sécurité sont dans ARCHITECTURE.md et SECURITY.md ; ce document fixe la façon d'écrire le code.

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

Corriger le formatage : `gofmt -w backend` et `npm --prefix frontend run format`. Un avertissement du linter est traité comme une erreur. Utiliser la version de Node de la CI (22) : une version plus ancienne fait échouer ESLint sans rapport avec le code.

### 2.1 Contrôles selon les fichiers modifiés

`scripts/verify.sh` s'exécute pour toute modification. S'y ajoutent, selon ce qui change :

| Modification | Contrôles supplémentaires obligatoires |
|---|---|
| Composant, page ou hook React | Règles du § 4 (en particulier § 4.7), thèmes clair et sombre et affichage mobile pour une page nouvelle ou modifiée |
| `frontend/package.json` ou `frontend/package-lock.json` | `npm audit` et `npm audit --omit=dev` (§ 6.2), OSV-Scanner (§ 6.3) |
| `backend/go.mod`, `backend/go.sum`, ou code Go qui appelle une dépendance pour la première fois | `govulncheck ./...` (§ 6.4), OSV-Scanner (§ 6.3) |
| Toute contribution avant une release, et au moins une fois par phase | `scripts/audit.sh` complet (§ 6.5) |

### 2.2 Rapporter un contrôle

Un contrôle est rapporté avec son résultat réel, dans l'une de ces catégories, jamais arrondi :

| Catégorie | Sens |
|---|---|
| **Réussi** | L'outil s'est exécuté jusqu'au bout et ne signale rien |
| **Problèmes détectés** | L'outil s'est exécuté et signale des erreurs ou des vulnérabilités : les lister avec leur gravité |
| **Outil indisponible** | L'outil n'est pas installé ou ne peut pas joindre sa base (réseau) : le dire et indiquer comment l'installer |
| **Non exécuté** | Le contrôle n'a pas été lancé (hors périmètre de la modification, ou impossible) : le dire et pourquoi |

Ne jamais écrire qu'un audit est propre s'il n'a pas été exécuté, s'il a échoué ou si son outil manque. L'absence de vulnérabilité détectée n'est pas une garantie de sécurité.

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
- Typographie : Instrument Sans pour l'interface et les chiffres (chiffres tabulaires), IBM Plex Mono réservée aux identifiants techniques (hôte, conteneur, image, chemin, adresse). Polices hébergées par l'application (`@fontsource`), la CSP interdisant tout chargement externe. Libellés en casse normale, jamais en capitales espacées ; texte de 12 px minimum.
- Palette neutre graphite : l'accent est le texte principal (bouton primaire clair sur fond sombre, et l'inverse) ; les teintes sont réservées aux séries de graphiques (`cpu`, `ram`, `net`) et aux états (`success`, `warning`, `danger`, `info`). Pas de dégradé, de halo ni d'ombre portée hors des fenêtres superposées.
- Accessibilité : chaque champ a un `label` associé ; messages d'erreur en `role="alert"`, confirmations en `role="status"` ; boutons d'icône avec `aria-label`.
- Thèmes clair et sombre et affichage mobile vérifiés pour toute nouvelle page.

### 4.6 Tests

- Vitest + React Testing Library ; tester le comportement visible (textes, rôles), pas l'implémentation.
- Tout schéma Zod, toute fonction de `src/lib/`, tout utilitaire de `src/hooks/` et tout client d'API ont des tests. La logique d'un hook de page se teste par les fonctions pures qu'il appelle ; un hook à comportement propre (enchaînement d'actions, navigation) se teste avec `renderHook`.

### 4.7 Bonnes pratiques React

Références : [react.dev/learn](https://react.dev/learn), [Règles de React](https://react.dev/reference/rules), [API de référence](https://react.dev/reference/react). Elles complètent les §§ 4.1 à 4.6 sans les remplacer.

**Composants et hooks**

- Composants fonctionnels uniquement ; pas de composant de classe.
- Règles des hooks : appelés au premier niveau d'un composant ou d'un hook `use…`, jamais dans une condition, une boucle, un rappel ni après un `return` anticipé. Contrôle : `eslint-plugin-react-hooks` (configuration recommandée, règles du compilateur React incluses).
- Rendu pur : un composant ou un hook ne produit aucun effet de bord pendant le rendu (requête, écriture dans le stockage, mutation d'une variable extérieure, lecture de `ref.current`). `StrictMode` reste activé (`src/main.tsx`) pour révéler les rendus impurs.
- Props et état sont immuables : jamais de mutation directe (`push`, `sort`, `splice`, affectation d'un champ) ; créer un nouvel objet ou tableau (`[...list].sort(…)`, `{ ...value, champ }`).
- Un composant a une responsabilité ; au-delà d'environ 200 lignes ou de plusieurs blocs indépendants, le découper en sous-composants du même fichier ou de son dossier de domaine. Pas d'abstraction générique avant un deuxième usage réel.

**Effets**

- `useEffect` sert uniquement à synchroniser avec un système extérieur (abonnement, minuterie, API du navigateur). Une valeur dérivée se calcule pendant le rendu (dans le hook, avec `useMemo` si le calcul est coûteux), jamais par un effet qui appelle `setState`. Une réaction à une action utilisateur va dans le gestionnaire d'événement ou le `onSuccess` d'une mutation.
- Tout effet qui s'abonne, démarre une minuterie ou lance une requête renvoie une fonction de nettoyage (désabonnement, `clearInterval`, `AbortController`).
- Tableaux de dépendances complets : `react-hooks/exhaustive-deps` ne se désactive pas. Si une dépendance pose problème, restructurer (déplacer la fonction dans l'effet, dériver la valeur) plutôt que l'omettre.

**Performances**

- `useMemo`, `useCallback` et `React.memo` seulement quand ils servent : calcul coûteux, référence stable exigée par un enfant mémorisé ou par les dépendances d'un autre hook. Ne pas les ajouter par principe.
- Éviter les re-rendus inutiles par la structure : état placé au plus près de son usage, clés de requête stables, objets et tableaux du modèle de vue mémorisés dans le hook quand ils sont recalculés à partir de données serveur.

**Listes**

- Chaque élément d'une liste a une `key` stable et unique issue des données (identifiant, nom d'unité, curseur) ; jamais l'index d'un tableau qui peut être trié, filtré ou modifié, jamais une valeur aléatoire.

**TypeScript**

- Types explicites pour les DTO (`src/lib/api/`), les props et les modèles de vue exportés (`ReturnType<typeof use…>`) ; unions de chaînes plutôt que `string` quand les valeurs sont connues ; `as const` pour les listes d'options.
- Pas d'assertion non nulle (`!`) ni de `as` pour faire taire le compilateur ; rétrécir le type (garde, `if`, valeur par défaut).

**États et erreurs**

- Toute donnée chargée traite ses trois états : chargement (`LoadingState`), erreur avec nouvel essai (`ErrorState`) et vide (`EmptyState`) ; toute mutation affiche son attente (`loading`, bouton désactivé) et son échec (`role="alert"`).
- Cas limites prévus : liste vide, valeur absente (`null` affiché « — », jamais 0), donnée périmée, droit insuffisant (action masquée, le serveur décide).

**Accessibilité**

- Éléments sémantiques : `button` pour une action, `Link` pour une navigation, tableaux avec en-têtes `TH`, titres hiérarchisés.
- Tout contrôle est utilisable au clavier et a un nom accessible (libellé visible, `label` associé ou `aria-label`) ; un bouton bascule porte `aria-pressed` ; un champ en erreur affiche le message près du champ.
- Ne pas transmettre une information par la couleur seule : un badge de sévérité ou d'état porte aussi un texte.

## 5. API et documentation

- `docs/API_CONTRACTS.md` est le contrat exact : toute route, paramètre, borne, DTO ou code d'erreur ajouté ou modifié l'est dans le même commit que le code.
- Un changement de comportement met à jour le document qui le décrit (METRICS_COLLECTION.md, DATA_MODEL.md, SECURITY.md…) dans le même commit.
- Nouvelle table ou colonne : nouvelle migration numérotée (`00N_description.sql`), jamais de modification d'une migration livrée ; schéma Ent régénéré depuis `backend/` avec `GOTOOLCHAIN=go1.26.11 go run entgo.io/ent/cmd/ent@v0.14.6 generate ./ent/schema`. Fixer `GOTOOLCHAIN` est nécessaire : `go run …@v0.14.6` est lancé hors du module et prendrait sinon le Go de base de la machine, trop ancien pour le `go.mod` du projet ; en cas d'échec, restaurer `ent/runtime.go` (`git checkout`), que le générateur laisse modifié.

## 6. Dépendances

### 6.1 Choix d'une dépendance

- Toute nouvelle dépendance (Go ou npm) est justifiée dans la description du commit : usage, alternatives écartées, impact (taille, maintenance, sécurité).
- Versions résolues figées par `frontend/package-lock.json` et `backend/go.sum`, toujours versionnés ; la CI installe avec `npm ci`. Version publiée depuis au moins deux semaines.
- Préférer la bibliothèque standard et le code interne court et testé à une dépendance pour quelques lignes.
- Avant d'ajouter une dépendance, vérifier sa provenance et sa légitimité : nom exact du paquet (pas de typosquat), dépôt source et éditeur reconnus, maintenance active (publication récente, anomalies traitées), licence compatible, nombre de dépendances transitives. Refuser un paquet abandonné, une version obsolète ou dépréciée, ou un paquet qui exécute des scripts d'installation sans raison.
- Retirer une dépendance devenue inutile dans le même commit que le dernier code qui l'utilisait.

### 6.2 npm audit

- Depuis `frontend/` : `npm audit` (toutes les dépendances) et `npm audit --omit=dev` (celles livrées dans l'interface). Les deux s'exécutent pour toute modification de `package.json` ou `package-lock.json`, et avant une release.
- Examiner les vulnérabilités directes et transitives (`npm audit` indique le chemin). Une vulnérabilité de gravité critique ou élevée bloque la livraison tant qu'elle n'est pas corrigée ou documentée (§ 6.6).
- Corriger par une montée de version ciblée de la dépendance concernée, puis relancer `scripts/verify.sh`. Interdit : `npm audit fix --force`, changer arbitrairement des versions pour faire disparaître une alerte, `overrides` sans justification écrite dans le commit.

### 6.3 OSV-Scanner

- Audit complémentaire de tous les manifestes et lockfiles du dépôt (`package-lock.json`, `go.mod`) : `osv-scanner scan source -r .` à la racine. Il ne remplace pas `npm audit` ni `govulncheck`, et réciproquement : chacun s'exécute.
- Examiner chaque résultat et sa gravité ; ne pas masquer un résultat (fichier d'ignorés, filtre) pour obtenir un audit propre. Un résultat ignoré l'est dans `osv-scanner.toml` à la racine avec sa raison et sa date de réexamen, et figure au § 6.6.
- Documentation : [google.github.io/osv-scanner](https://google.github.io/osv-scanner/).

### 6.4 govulncheck

- Depuis `backend/` : `govulncheck ./...`, pour toute modification de `go.mod` ou `go.sum`, tout premier appel à une dépendance, toute montée de version de Go et avant une release.
- Il signale les vulnérabilités réellement atteignables depuis le code (chemin d'appel) : examiner ce chemin. Une vulnérabilité atteignable est corrigée (montée de version de la dépendance ou de Go, puis `govulncheck` et `scripts/verify.sh` relancés) ; une vulnérabilité seulement présente dans le module, non atteignable, est notée et réexaminée à la mise à jour suivante.
- Documentation : [go.dev/doc/security/vuln](https://go.dev/doc/security/vuln/).

### 6.5 Script d'audit

- `scripts/audit.sh` enchaîne `npm audit`, `npm audit --omit=dev`, OSV-Scanner et `govulncheck` et rapporte chacun dans les catégories du § 2.2. Code de sortie : 0 si tout est réussi, 1 si des vulnérabilités sont détectées, 2 si un outil est indisponible ou a échoué. Il interroge des bases en ligne ; il n'est donc pas inclus dans `scripts/verify.sh`, qui reste exécutable hors ligne.
- CI : le workflow `.github/workflows/audit.yml` le lance à chaque push, chaque pull request, chaque lundi et à la demande ; `release.yml` le lance avant de publier. Tout code de sortie non nul fait échouer le job : une vulnérabilité, un outil absent ou une base injoignable bloquent la release. Les outils y sont installés par `scripts/install-audit-tools.sh` à des versions figées, publiées depuis au moins deux semaines ; changer ces versions suit le § 6.1.
- Installation des outils (outils de développement, hors dépendances du projet) : `go install golang.org/x/vuln/cmd/govulncheck@latest`, lancé hors de `backend/` pour être compilé avec le Go le plus récent de la machine (un `govulncheck` compilé avec un Go plus ancien que le `go` du `PATH` ne sait pas charger le code et échoue) ; le script l'exécute avec la version de Go de `backend/go.mod`, celle de la CI et des releases, pour que la bibliothèque standard analysée soit celle livrée ; OSV-Scanner par `brew install osv-scanner` (macOS) ou `go install github.com/google/osv-scanner/v2/cmd/osv-scanner@latest`.

### 6.6 Vulnérabilités non corrigées

Une vulnérabilité qui ne peut pas être corrigée immédiatement (pas de version corrigée, correction incompatible) est documentée dans `docs/SECURITY.md` § « Vulnérabilités connues des dépendances » : identifiant (CVE, GHSA ou GO-), paquet et version, gravité, atteignabilité, mesure de réduction, date de réexamen. Elle n'est jamais passée sous silence.

## 7. Git

- Branche de travail `dev` ; `main` reçoit des fusions de `dev`.
- Chaque push sur `main` publie une release (`.github/workflows/release.yml`) : vérification complète, archives Linux `amd64` et `arm64`, `SHA256SUMS`, tag `vMAJEUR.MINEUR.CORRECTIF`. La version se déduit des Conventional Commits depuis le dernier tag (`scripts/release/next-version.sh`) : `type!:` ou `BREAKING CHANGE:` → majeure, `feat` → mineure, le reste → correctif ; la première est `2.3.0`. Le type d'un commit compte donc : un `feat` qui n'ajoute rien de visible fausse la version.
- La page de présentation `site/` est publiée sur GitHub Pages (`.github/workflows/pages.yml`) à chaque push sur `dev` qui la modifie ; elle est statique, sans dépendance à construire, et ne doit décrire que des fonctions livrées.
- La version est injectée à la construction (`-X main.version`, `VITE_APP_VERSION`) ; ne jamais l'écrire en dur dans le code.
- Messages au format Conventional Commits, en anglais : `type(scope): résumé à l'impératif`, avec `type` ∈ `feat`, `fix`, `refactor`, `perf`, `test`, `docs`, `chore`, `build` ; corps expliquant le pourquoi.
- Un commit = un changement cohérent, avec ses tests et sa documentation, qui passe `scripts/verify.sh`.
- Ne jamais versionner : `.env`, bases SQLite (`*.db`, `-wal`, `-shm`), binaires, `dist/`, secrets.

## 8. Sécurité du code

Ces règles valent pour toutes les technologies du projet ; les exigences propres au produit sont dans SECURITY.md.

- Aucun secret (mot de passe, clé d'API, jeton, clé de chiffrement) dans le code, les tests, la configuration versionnée ou les messages de commit ; les secrets viennent de systemd (`ImportCredential=`) ou de fichiers hors dépôt. Les tests utilisent des valeurs fictives générées.
- Aucune donnée sensible dans les journaux, les erreurs renvoyées ni l'interface : mot de passe, cookie, jeton, secret, variable d'environnement, ligne de commande d'un processus (§ 3.5, API_CONTRACTS.md § 8).
- Toute donnée externe est validée côté serveur avant usage : requête HTTP (corps strict, paramètres bornés), réponse Docker ou D-Bus, sortie de `journalctl`, fichier lu. La validation de l'interface (Zod) n'est jamais une protection.
- Moindre privilège : le Manager ne tourne jamais en root ; chaque accès (groupe, socket, règle polkit, permission de rôle) est le plus étroit possible et justifié dans SECURITY.md.
- Ne jamais désactiver une protection pour contourner un problème : contrôle d'origine, vérification TLS, durcissement systemd, règle ESLint, `-race`, test qui échoue. Corriger la cause.
- Ne jamais masquer une erreur, une alerte de sécurité ou un résultat d'audit sans justification écrite (commentaire au plus près du code, ou § 6.6).
