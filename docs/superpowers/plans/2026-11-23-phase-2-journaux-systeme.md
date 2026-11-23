# Phase 2 — Journaux système : plan d'implémentation

> **Pour les agents :** sous-skill requis : `superpowers:executing-plans` ou `superpowers:subagent-driven-development`. Chaque tâche suit le cycle test qui échoue → implémentation → `scripts/verify.sh` → commit.

**Objectif :** livrer `SPEC_ADMINISTRATION.md` § 4 (module J) : consultation en lecture seule du journal systemd, filtres, pagination par curseur, suivi en direct, inspecteur masqué, synthèse sur 24 h, occupation disque, export.

**Architecture :** les filtres, leur validation, le masquage, la recherche de texte et le calcul de la synthèse sont des fonctions pures du domaine. Un adaptateur `journalctl` construit les arguments à partir de filtres déjà validés, lance le programme sans shell avec un environnement vide, lit la sortie JSON en flux et borne durée, taille de ligne et nombre de processus. Un `JournalUsecase` pagine, filtre le texte, met la synthèse en cache. Aucune table : le journal reste la source.

**Prérequis :** phase 1 (rôles). Indépendante de 1C.

**Écart assumé au format habituel :** comme pour 1B et 1C, ce plan fixe la conception, les interfaces et les cas de test ; le code est écrit test d'abord pendant l'exécution.

## Comportement de `journalctl` vérifié (systemd 255, fichier journal de test)

| Besoin | Arguments | Constat |
|---|---|---|
| Page la plus récente | `-r -n N` | Plus récentes d'abord |
| Page plus ancienne | `-r --after-cursor=C -n N` | Entrées antérieures à `C`, `C` exclue |
| Suivi (plus récentes) | `--after-cursor=C -n N` | Entrées postérieures à `C`, ordre croissant |
| Une entrée | `--cursor=C -n 1` | `C` incluse |
| Intervalle | `--since=@s.µs --until=@s.µs` | `--until` est **inclusif** : `to` − 1 µs pour obtenir `[from, to)` |
| Démarrages | `--list-boots -o json` | `index`, `boot_id`, `first_entry`, `last_entry` (µs) |
| Authentification | `--facility=auth,authpriv` | Facilités 4 et 10 |

## Décisions de conception

| Sujet | Choix | Motif |
|---|---|---|
| Rôles | Tous les rôles connectés (consultation, § 3.2) | Spécification ; le masquage protège les champs sensibles |
| Programme | `journalctl` trouvé dans le `PATH` du service au démarrage ; absent → module indisponible (`503 JOURNAL_UNAVAILABLE`), le reste du Manager fonctionne | § 2.3 |
| Exécution | `exec` sans shell, arguments sous la forme `--option=valeur` (une valeur ne peut pas devenir une option), environnement réduit à `LANG=C.UTF-8`, `SYSTEMD_COLORS=0`, `SYSTEMD_PAGER=` : les secrets du Manager ne sont pas transmis | § 2.2, § 2.3 |
| Bornes | 10 s par appel, 4 processus à la fois (au-delà : attente bornée par la requête), ligne JSON de 1 MiB au plus (au-delà : entrée ignorée et comptée), message tronqué à 8 Kio dans les listes | § 2.3 |
| Accès complet | Le compte doit appartenir à `systemd-journal` (ou `adm`, ou être root) : contrôlé au démarrage par les groupes du processus et signalé (`fullAccess`) ; sinon `journalctl` ne montre que les entrées du compte | Diagnostic fiable sans analyser `stderr` |
| Liste | `-o json --all --output-fields=` limité aux champs affichés ; 500 entrées par page au plus (défaut 200) ; toujours renvoyées des plus récentes aux plus anciennes | Volume borné |
| Recherche de texte | Sous-chaîne insensible à la casse faite par le Manager sur `MESSAGE`, l'unité et l'identifiant ; au plus 5 000 entrées examinées par requête, le curseur rendu permet de continuer (`scanLimitReached`) | Jamais d'expression transmise à `journalctl` |
| Curseurs | Opaques pour l'interface, validés (`^[a-z]=[0-9a-f]+(;[a-z]=[0-9a-f]+)*$`, 512 octets au plus) | Aucun paramètre libre |
| Unités | Liste par `journalctl -F _SYSTEMD_UNIT` (1 000 au plus, triée) en attendant le module S ; nom validé par motif | § 4 « liste issue du module S » : remplacée en phase 3 |
| Masquage | `_CMDLINE` et tout champ dont le nom contient `PASSWORD`, `TOKEN` ou `SECRET` (casse ignorée) → valeur remplacée, `masked: true` ; aussi dans l'export JSON | § 4 |
| Données binaires | Tableau d'octets : converti en texte UTF-8 valide pour `MESSAGE`, sinon « [données binaires, N octets] » ; champ multivalué : valeurs jointes par un saut de ligne | Format JSON de `journalctl` |
| Synthèse | 96 tranches de 15 min × 8 priorités sur les 24 h écoulées, `--output-fields=PRIORITY`, 200 000 entrées au plus (`truncated`), mise en cache 60 s ; occupation disque par `--disk-usage` | § 4, coût borné |
| Export | `.log` (format court produit par le Manager, après masquage) ou `.json` (tableau d'entrées masquées), 50 000 entrées au plus, envoyé en flux ; mêmes filtres que la liste | § 4 |
| Suivi en direct | L'interface interroge toutes les 2 s avec `after` ; TanStack Query suspend l'interrogation quand l'onglet est masqué | § 4, pas de WebSocket |
| Déploiement | `SupplementaryGroups=systemd-journal` dans l'unité du Manager | § 2.1 |

## Routes

| Route | Rôle |
|---|---|
| `GET /api/v1/journal/entries` | Filtres `source`, `unit`, `priority`, `boot`, `from`, `to`, `q`, `limit`, et `before` ou `after` (curseur) → `{ entries, olderCursor, newerCursor, scanned, scanLimitReached, skipped }` |
| `GET /api/v1/journal/entry?cursor=` | Tous les champs d'une entrée, masqués |
| `GET /api/v1/journal/boots` | Démarrages connus, du plus récent au plus ancien |
| `GET /api/v1/journal/units` | Unités présentes dans le journal |
| `GET /api/v1/journal/summary` | Synthèse 24 h et occupation disque, `fullAccess` |
| `GET /api/v1/journal/export?format=log\|json` | Mêmes filtres que la liste, fichier joint |

## Tâches

1. **Domaine** : filtres et validation (source, priorité, unité, démarrage, intervalle, curseur, limite), masquage, recherche, entrée affichée, synthèse (tranches et priorités), format court.
2. **Adaptateur `journalctl`** : construction des arguments (tests de table), lecture JSON en flux (octets, multivaleurs, `null`, ligne trop longue), groupes, démarrages, unités, occupation disque ; tests d'intégration contre le vrai `journalctl` sur un fichier journal fabriqué par `systemd-journal-remote` (ignorés si absents).
3. **Usecase** : pagination dans les deux sens, recherche bornée, détail, cache de la synthèse avec horloge injectable, export borné.
4. **HTTP et contrat** : routes, validation des paramètres, erreurs, en-têtes d'export ; `API_CONTRACTS.md`, `SECURITY.md`, `ARCHITECTURE.md`.
5. **Interface** : page « Journaux système » (filtres, synthèse ECharts, table, chargement des plus anciennes, suivi en direct, inspecteur, export), entrée de navigation activée ; hook `useJournalPage`, fonctions pures dans `src/lib/journal.ts`, schéma Zod des filtres.
6. **Déploiement et état** : unité systemd, README, spécifications, tests ; parcours réel dans Chromium avec un `journalctl` lisant un fichier journal fabriqué.
