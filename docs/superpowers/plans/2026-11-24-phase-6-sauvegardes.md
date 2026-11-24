# Phase 6 — Sauvegardes restic (module B) : plan d'implémentation

> **Pour les agents :** sous-skill requis : `superpowers:executing-plans` ou `superpowers:subagent-driven-development`. Chaque tâche suit le cycle test qui échoue → implémentation → `scripts/verify.sh` → commit en anglais (Conventional Commits) avec ses documents.

**Objectif :** livrer `SPEC_ADMINISTRATION.md` § 7 : le Manager configure et pilote restic, qui s'exécute uniquement dans l'unité root fournie et durcie `homelab-backup@.service` ; dépôts, sources, exclusions, sauvegardes planifiées et manuelles, historique, rétention, vérification, instantanés et restauration en quarantaine ; règle d'alerte « Sauvegarde ».

**Prérequis :** phase 1C (coffre de secrets), phase 4 (alertes), phase 5 et module V (démarrage d'une unité par D-Bus et polkit). restic : paquet Debian 13 `restic` 0.18.0.

**Statut :** D-12 tranchée le 24 novembre 2026 (option A : le Manager planifie). Incrément 6A livré sauf la règle d'alerte « Sauvegarde » (tâche 7), à faire ensuite ; 6B à venir.

## Découpage

- **6A — Sauvegarder** : dépôt (initialisation, mot de passe affiché une fois), sources et exclusions, programme d'entrée de l'unité, sauvegarde manuelle et planifiée, historique des exécutions, instantanés, règle d'alerte « Sauvegarde » et règle par défaut « aucune sauvegarde réussie depuis 26 h », page « Sauvegardes ».
- **6B — Entretenir et restaurer** : rétention `forget --prune`, `check --read-data-subset` hebdomadaire, diff entre deux instantanés, restauration simulée puis en quarantaine, copies vers d'autres dépôts (`restic copy`), état du verrou et déverrouillage forcé.

## Décisions de conception

| Sujet | Choix proposé | Motif |
|---|---|---|
| Programme d'entrée | Binaire Go `homelab-backup-runner`, fourni avec la release comme le proxy Docker, lancé par `homelab-backup@.service` (`ExecStart=/usr/local/bin/homelab-backup-runner %i`). Il lit le fichier de travail, le valide de nouveau (schéma strict, opération = instance de l'unité, chemins absolus normalisés sans `..`, motifs d'exclusion bornés), construit les arguments restic selon un schéma fixe, lance `restic --json` sans shell et écrit le résultat | Écart au § 7.1 (« script d'entrée ») : un binaire Go se teste et valide mieux qu'un script ; aucune donnée ne devient une option restic libre |
| Échange de fichiers | Comme le module V, hors du répertoire d'état du Manager (0700) : `/var/lib/homelab-backup/jobs/` (écrit par le Manager, lu par l'unité) et `/var/lib/homelab-backup/runs/` (écrit par l'unité, lu par le Manager), créés par `tmpfiles.d` | Écart au § 7.1 (`/var/lib/homelab-manager/backup/`), même motif que le module V |
| Mot de passe du dépôt | Généré par le Manager à l'initialisation (32 octets aléatoires, encodés), affiché une seule fois avec la consigne d'en garder une copie hors ligne, scellé dans le coffre (§ 2.5). Pour chaque exécution, le Manager écrit un fichier de mot de passe à usage unique (0640, groupe de l'unité) que le programme d'entrée lit puis supprime (`RESTIC_PASSWORD_FILE`). Sans clé maître, les sauvegardes sont indisponibles | § 7.2 ; le mot de passe ne figure jamais dans un argument ni dans un journal |
| Dépôts en 6A | Répertoire local ou monté, rest-server (HTTPS), S3 compatible (dont B2 par son API S3) ; identifiants S3 et rest-server scellés dans le coffre et transmis par le même fichier à usage unique. SFTP reporté (clé SSH de root à gérer hors du Manager) | § 7.2 ; SFTP demande une décision sur la gestion des clés |
| Sources | Chemins absolus choisis par un administrateur, normalisés, sans `..`, 50 au plus ; étiquettes | § 7.3 |
| Exclusions | Motifs glob restic, 500 au plus, 4 Kio chacun, syntaxe vérifiée par le Manager ; options `--exclude-caches` et `--exclude-if-present` (nom de fichier fixe) | § 7.3 |
| Planification (D-12) | Le Manager planifie (option A), vérification toutes les 30 s ; une sauvegarde manquée pendant un arrêt est lancée une fois, 2 min après le redémarrage | Tranchée le 24 novembre 2026 |
| Exécution | Une opération à la fois (verrou du Manager, plus le verrou de restic) ; 6 h au plus par sauvegarde (`TimeoutStartSec=`) ; progression lue dans les lignes `status` de `restic --json` | Bornes du § 2.2 |
| Unité | `homelab-backup@.service`, root, `Type=oneshot`, `ProtectSystem=strict`, `ProtectHome=read-only`, `PrivateTmp`, `NoNewPrivileges`, `ReadWritePaths=` limité au cache restic, à `runs/` et au répertoire de quarantaine, réseau autorisé, `CPUWeight=idle`, `IOSchedulingClass=idle`, `Nice=10`. Règle polkit : verbe `start` sur les seules instances `homelab-backup@backup.service`, `@check`, `@forget-prune`, `@snapshots`, `@init` (6A), `@restore`, `@diff`, `@copy` (6B) | § 2.1 |
| Données | Migration `010_backups.sql` : `backup_repositories`, `backup_sources`, `backup_settings` (exclusions, planification, rétention), `backup_runs` (opération, déclencheur, demandeur, état, cause d'échec, début, fin, statistiques de restic, identifiant d'instantané) | § 7.4 |
| Rôles | `admin` : dépôts, sources, exclusions, planification ; `operator` : lancer une sauvegarde ; tous : consulter. Nouvelles permissions `backups:manage` et `backups:run` | § 3.2 |
| Alerte | Type de règle `backup` : dernier succès plus ancien que N heures, ou dernière exécution en échec ; règle par défaut « aucune sauvegarde réussie depuis 26 h » | § 6.1 |
| Pré-traitements | Aucun en 6A ; D-3 reste ouverte | Règle 3 |

## D-12 — Planification des sauvegardes (tranchée : option A)

Le § 7.1 prévoit un timer `homelab-backup-schedule.timer` dont l'expression `OnCalendar` est « générée à partir des réglages ». Le Manager, non root, ne peut pas écrire ce timer. Deux options :

- **A (proposée) — le Manager planifie** : l'horaire se règle dans l'interface et s'applique aussitôt ; le Manager démarre `homelab-backup@backup.service` par D-Bus à l'heure dite, comme il le fait pour le module V. Inconvénient : aucune sauvegarde planifiée pendant un arrêt du Manager ; une sauvegarde manquée est lancée à son redémarrage.
- **B — timer systemd généré** : une sous-commande `homelab-manager backup-timer` écrit le timer à partir de la configuration, comme `polkit-rule` ; l'administrateur l'installe en root. La sauvegarde a lieu même si le Manager est arrêté, mais chaque changement d'horaire passe par la ligne de commande, et l'interface ne fait qu'afficher l'horaire.

## Routes (6A)

| Route | Rôle |
|---|---|
| `GET /api/v1/backups/summary` | Tous |
| `GET /api/v1/backups/runs`, `GET /api/v1/backups/runs/{id}` | Tous |
| `GET /api/v1/backups/snapshots` | Tous |
| `POST /api/v1/backups/runs` `{ "operation": "backup" }` | `operator` |
| `GET`/`PUT /api/v1/backups/repository`, `POST /api/v1/backups/repository/init` | `admin` |
| `GET`/`PUT /api/v1/backups/sources`, `GET`/`PUT /api/v1/backups/settings` | `admin` (lecture : tous) |

## Tâches (6A)

1. Domaine : dépôts, sources, exclusions, planification, exécutions, validation. Tests.
2. Programme d'entrée `homelab-backup-runner` : lecture et validation du fichier de travail, arguments restic, décodage de `--json`, résultat. Tests avec un faux restic, puis avec le vrai restic sur un dépôt local temporaire.
3. Migration `010` et adaptateur SQLite. Tests sur SQLite réel.
4. Adaptateur d'exécution : fichiers de travail et de mot de passe, démarrage de l'unité (contrôleur de la phase 5), lecture du résultat.
5. Usecase : verrou, planification (selon D-12), sauvegarde manuelle auditée, historique.
6. HTTP, puis interface : page « Sauvegardes » (état, dernier succès, prochaine sauvegarde, historique, instantanés, bouton « Sauvegarder maintenant ») et réglages administrateur.
7. Règle d'alerte `backup` et sa règle par défaut.
8. Unités, `tmpfiles.d`, règle polkit, documentation (API_CONTRACTS, DATA_MODEL, SECURITY, README, SPEC_ADMINISTRATION, TESTING), recette dans la VM Debian.

## Risques à documenter (SECURITY.md)

- L'unité s'exécute en root et lit tout chemin source qu'un administrateur choisit : un compte `admin` du Manager peut donc copier n'importe quel fichier lisible par root vers un dépôt distant. C'est inhérent au § 7.3 ; l'unité ne peut toutefois rien écrire hors du cache, de `runs/` et de la quarantaine.
- Un Manager compromis peut faire de même en écrivant un fichier de travail : le programme d'entrée valide le schéma, mais pas l'intention.

## Bilan de l'incrément 6A (24 novembre 2026)

- Livré : domaine, `homelab-backup-runner`, migration `010` et dépôt SQLite, exécuteur (contrôleur D-Bus de la phase 5), usecase (configuration, initialisation, sauvegarde manuelle, planification et rattrapage), routes, page « Sauvegardes », unité, `tmpfiles.d`, règle polkit, documentation.
- Écarts : `ReadWritePaths=` de l'unité étendu aux points de montage usuels (`/mnt`, `/media`, `/srv`, `/var/backups`) pour qu'un dépôt local soit inscriptible avec `ProtectSystem=strict` ; capacités limitées à `CAP_DAC_READ_SEARCH` et `CAP_DAC_OVERRIDE` ; un dépôt existant ne peut pas encore être rattaché (6B).
- Recette dans la VM Debian 13 (restic 0.18.0, systemd, polkitd 126) : test réel du programme d'entrée (initialisation, sauvegarde avec exclusions, mauvais mot de passe) ; parcours par l'API : configuration d'un dépôt local sous `/srv`, initialisation, sauvegarde avec exclusion de `*.tmp`, instantané listé, fichiers de secrets supprimés, résultats lisibles par le groupe du Manager, prochaine sauvegarde calculée dans le fuseau de la machine.
- Reste : règle d'alerte `backup` et règle par défaut « aucune sauvegarde réussie depuis 26 h » ; vérification visuelle de la page (thèmes, mobile) ; rest-server et S3 réels.
