# Feuille de route — Homelab Manager v2.1 → v3

Date : 23 novembre 2026. Référence : `docs/SPEC_ADMINISTRATION.md` (v3.0 brouillon), `docs/PRODUCT_SPEC.md` (v2.1), `AGENTS.md`.

Ce document découpe tout le travail restant en incréments livrables. Chaque incrément a ensuite son plan détaillé (TDD, tâche par tâche) dans `docs/superpowers/plans/`. Deux sont rédigés : [phase 1A — rôles et comptes multiples](./2026-11-23-phase-1a-roles-comptes.md) et [module M — redémarrer et éteindre la machine](./2026-11-23-module-m-alimentation.md). Les suivants seront détaillés un par un, au démarrage de chaque phase, à partir du code alors en place.

## 1. Point de départ

La v2.1 est livrée et `scripts/verify.sh` passe (gofmt, vet, tests Go avec et sans `-race`, Prettier, ESLint, TypeScript, Vitest, build) :

- Backend Go hexagonal (`domain`, `usecase`, `ports`, `adapters`, `delivery/http`), Chi v5, Ent, SQLite `modernc.org/sqlite`, migrations `001`–`002`.
- Authentification locale à compte unique (`admin`), sessions par cookie, Argon2id, contrôle d'origine, limite de débit par IP, audit de la rétention.
- Collecte système et Docker (proxy Unix en liste blanche), historique brut et agrégé, purge horaire par lots.
- Interface React : tableau de bord, machine, conteneurs, images, volumes, réseaux, stockage, métriques, réseau, événements, paramètres. Six entrées « à venir » dans la navigation.

## 2. Règles communes à chaque incrément

Chaque incrément, quel qu'il soit :

1. Respecte les règles intangibles de `SPEC_ADMINISTRATION.md` § 1 : aucun terminal, shell ni SSH ; comptes du Manager uniquement ; aucune exécution de code fourni par l'utilisateur ; Manager jamais root ; pas de Prometheus ni de base externe ; pas d'action Docker mutatrice.
2. Ajoute ses tables par une **nouvelle** migration numérotée, sans modifier les migrations livrées, et régénère Ent (`go run entgo.io/ent/cmd/ent@v0.14.6 generate ./ent/schema`).
3. Met à jour, **dans le même commit que le code** : `API_CONTRACTS.md` (routes, DTO, codes d'erreur), `DATA_MODEL.md` (tables), `SECURITY.md` (risques), `TESTING.md` (couverture), et l'état d'avancement dans `PRODUCT_SPEC.md` § 6, `README.md` et `SPEC_ADMINISTRATION.md`.
4. Livre ses tests avec le code : domaine et usecases avec faux objets écrits à la main, adaptateur SQLite sur fichier temporaire réel, routes avec `httptest`, schémas Zod, fonctions de `src/lib/` et clients d'API côté interface.
5. Retire la mention « à venir » de son entrée de navigation seulement quand le module est utilisable.
6. Passe `scripts/verify.sh` avant chaque commit (Conventional Commits en anglais).
7. Toute dépendance nouvelle est justifiée dans le commit et figée à une version publiée depuis au moins deux semaines.

## 3. Vue d'ensemble

| Incrément | Contenu | Prérequis | Migration | Taille |
|---|---|---|---|---|
| **1A** | Rôles, comptes multiples, mot de passe initial à changer, autorisation serveur | — | `003_user_accounts.sql` | M |
| **M** | Redémarrer et éteindre la machine (logind + polkit), désactivé par défaut | 1A | — | S |
| **1B** | Sessions (liste, révocation, inactivité), verrouillage de compte, politique de sécurité, journal d'audit consultable et exportable | 1A | `004_sessions_security.sql` | M |
| **1C** | Coffre de secrets (AES-256-GCM, `systemd-creds`), TOTP, codes de secours, connexion en deux étapes, 2FA obligatoire pour les administrateurs | 1B | `005_second_factor.sql` | L |
| **2** | Journaux système (lecture `journalctl`) | 1A, groupe `systemd-journal` | — | M |
| **3** | Services système en lecture (D-Bus), événements `service.failed` / `service.recovered` | 1A, D-Bus | — | M |
| **4** | Alertes et notifications : règles, cycle de vie, silences, canaux | 1C (secrets), 3 | `006_alerts.sql` | L |
| **5** | Actions sur les services par polkit (`start`, `stop`, `restart`, `reload`) | 1B (audit), 3 | — | M |
| **6** | Sauvegardes restic : dépôts, sources, exclusions, rétention, restauration en quarantaine | 1C, 4, 5 | `007_backups.sql` | XL |
| **7** | Tâches planifiées en lecture (Cron et timers) | 2, 3 | — | S |
| **8** | WebAuthn (passkeys, FIDO2) et jetons d'API en lecture | 1C, HTTPS | `008_webauthn_tokens.sql` | L |
| *Option* | Dettes v2.1 : pagination par curseur, inventaire complet images/volumes/réseaux | Décision Q-4 | selon choix | M |

Le coffre de secrets (§ 2.5 de la spécification) est avancé en **1C** et non en phase 4 : les secrets TOTP doivent être chiffrés (§ 3.6), ce qui en fait le premier consommateur.

Ordre de réalisation : 1A → M → 1B → 1C → 2 → 3 → 4 → 5 → 6 → 7 → 8. Les phases 2 et 3 sont indépendantes de 1B et 1C et peuvent être avancées si le second facteur n'est pas prioritaire ; 7 peut suivre 2 et 3 sans attendre 4–6.

## 4. Détail des incréments

### 1A — Rôles et comptes multiples (plan détaillé rédigé)

- Rôles `viewer`, `operator`, `admin` (§ 3.2) ; politique rôle → permission dans le domaine, appliquée par un middleware HTTP.
- Comptes : identifiant `^[a-z0-9._-]{1,64}$`, nom affiché, rôle, actif ou désactivé, dernière connexion et IP.
- Création par un administrateur avec mot de passe initial (12 caractères au moins) à changer à la première connexion : toutes les routes sauf déconnexion et changement de mot de passe répondent `403 PASSWORD_CHANGE_REQUIRED` en attendant.
- Désactivation (révoque les sessions), changement de rôle, suppression (révoque les sessions, conserve l'audit) ; refus `409 LAST_ADMIN` pour le dernier administrateur actif.
- Rétention réservée aux administrateurs.
- Interface : page « Utilisateurs & accès » (administrateurs), écran de changement de mot de passe imposé, paramètres en lecture seule pour les autres rôles.
- Plan : [2026-11-23-phase-1a-roles-comptes.md](./2026-11-23-phase-1a-roles-comptes.md).

### M — Redémarrer et éteindre la machine (plan détaillé rédigé)

- Demandé le 23 novembre 2026 ; spécifié dans `SPEC_ADMINISTRATION.md` § 8 bis (ajout du même jour).
- `systemd-logind` par D-Bus (`Reboot`, `PowerOff`), règle polkit fournie n'accordant que `reboot` et `power-off` (et `-multiple-sessions`), jamais `-ignore-inhibit` ; réglage `power_actions_enabled` à `false` par défaut.
- Rôle `admin` uniquement (décision D-7, tranchée), pas d'arrêt programmé ni annulable (décision D-8, tranchée), confirmation par saisie du nom de la machine vérifiée par le serveur, audit écrit **avant** la demande, une seule demande acceptée.
- Introduit le port `AuditRecorder` (audit hors transaction métier), réutilisé ensuite par 1B.
- Dépendance `github.com/godbus/dbus/v5` v5.2.4 (publiée le 13 octobre 2026).
- Interface : carte « Alimentation » de la page Machine, visible des administrateurs.
- À tester sur une machine virtuelle Debian 13 jetable, jamais sur la machine de développement.
- Plan : [2026-11-23-module-m-alimentation.md](./2026-11-23-module-m-alimentation.md).

### 1B — Sessions, verrouillage, politique, audit (livrée)

- Plan : [2026-11-23-phase-1b-sessions-securite.md](./2026-11-23-phase-1b-sessions-securite.md).

- Migration : `sessions.last_seen_at`, `sessions.auth_method` ; tables `security_policy` (singleton), `login_failures`.
- Sessions : liste des siennes (navigateur, IP, méthode, dernière activité), révocation unitaire et « toutes sauf celle-ci » ; un administrateur voit et révoque celles de tous. `last_seen_at` mis à jour au plus une fois par minute pour limiter les écritures SQLite.
- Inactivité : expiration après N minutes (défaut 60) en plus des 12 h fixes.
- Verrouillage : 5 échecs consécutifs → compte verrouillé 15 min, en plus de la limite par IP ; réponse toujours `401 INVALID_CREDENTIALS` (pas d'énumération) avec coût Argon2id constant.
- Réseaux autorisés (CIDR) : refus de toute modification qui exclurait l'adresse de l'administrateur courant.
- Audit : réutiliser le port `AuditRecorder` (introduit par M) pour tracer aussi les refus (`outcome = denied`), route de consultation filtrable (acteur, cible, action, période) et export CSV/JSON borné ; index `audit_logs (created_at DESC)`.
- Interface : onglets Sessions, Politique et Journal d'audit dans « Utilisateurs & accès ».
- Risques : charge d'écriture de `last_seen_at`, horloge injectable pour les fenêtres de verrouillage.

### 1C — Coffre de secrets et second facteur

- Coffre : clé maître lue depuis `$CREDENTIALS_DIRECTORY` (`LoadCredentialEncrypted=` dans l'unité systemd), AES-256-GCM, nonce aléatoire, empreinte SHA-256 tronquée pour l'affichage ; démarrage en mode dégradé explicite si la clé manque (fonctions 2FA indisponibles, supervision intacte). Documentation `systemd-creds encrypt` dans le README.
- TOTP interne (RFC 6238, SHA-1, 6 chiffres, 30 s, ±1 période, anti-rejeu) avec vecteurs de test de la RFC ; enrôlement par QR code (bibliothèque de génération à justifier, ou SVG produit côté serveur).
- 10 codes de secours affichés une fois, stockés hachés.
- Connexion en deux étapes : jeton d'étape intermédiaire court (5 min), puis `POST /auth/login/second-factor`.
- Politique « 2FA obligatoire pour les administrateurs » (défaut : désactivée tant qu'aucun facteur n'est enrôlé).
- Tables : `totp_secrets`, `recovery_codes`, `secrets` (si le coffre est générique).

### 2 — Journaux système

- Adaptateur `journalctl` : `exec` sans shell, arguments construits depuis des options validées (`--since`, `--until`, `--priority`, `--unit`, `--boot`, `--lines`, curseurs), sortie JSON bornée en taille et en lignes, délai 10 s.
- Recherche de sous-chaîne faite par le Manager (jamais une regex transmise), 500 entrées par page, pagination par curseur journald, suivi par interrogation toutes les 2 s (arrêt quand l'onglet est masqué).
- Masquage de `_CMDLINE` et des champs contenant `PASSWORD`, `TOKEN`, `SECRET`.
- Synthèse 24 h par tranches de 15 min, `--disk-usage`, export `.log` / `.json` (50 000 entrées au plus).
- Déploiement : ajouter `homelab-manager` au groupe `systemd-journal` (README, unité).
- Tests : fixtures JSON de `journalctl`, construction des arguments, bornes, masquage.

### 3 — Services système en lecture

- Dépendance `github.com/coreos/go-systemd/v22/dbus` (justifiée § 2.4 de la spécification).
- Collecte toutes les 30 s par une goroutine détenue par un usecase (arrêt par contexte, délai par cycle) ; inventaire services, timers, sockets avec états, PID, mémoire, CPU, dernier changement.
- Événements `service.failed` (sévérité `error`) et `service.recovered` dans la table `events` existante.
- Détail d'unité : `FragmentPath`, `ExecStart` affiché tel quel, dépendances, limites, slice, 50 dernières lignes du journal (phase 2).
- Tests : port D-Bus simulé, transitions d'état.

### 4 — Alertes et notifications

- Moteur évalué toutes les 15 s sur les derniers échantillons (aucune requête historique lourde) ; six types de règles (§ 6.1) ; règles par défaut désactivables.
- Cycle `firing` → `acknowledged` → `resolved`, silences (15 min à 7 jours), indicateurs.
- Canaux : Telegram, Discord, Gotify, ntfy, SMTP, webhook (signature HMAC-SHA256 optionnelle) ; secrets dans le coffre (1C) ; bouton « Tester » ; regroupement 1/min/canal ; renotification toutes les N minutes.
- Seule communication sortante du Manager : délais courts, échecs sans effet sur la supervision, file bornée.
- Tables : `alert_rules`, `alerts`, `alert_silences`, `notification_channels`.
- Retire « Alertes » de « à venir ».

### 5 — Actions sur les services

- Règle polkit générée depuis `systemd_manageable_units` (`/etc/polkit-1/rules.d/50-homelab-manager.rules`), verbes `start`, `stop`, `restart`, `reload` seulement.
- Validation par motif **et** liste blanche, une action par unité à la fois (`409 ACTION_IN_PROGRESS`), 30 actions/min/utilisateur, suivi jusqu'à l'état stable (90 s max), confirmation, audit réussi ou non.
- Rôle `operator` au moins ; nouvelle permission `services:operate` dans le domaine.
- Action « Redémarrer » depuis une alerte (phase 4).

### 6 — Sauvegardes restic

- Unités fournies `homelab-backup@.service` (durcies) et `homelab-backup-schedule.timer` ; fichiers de travail JSON dans `/var/lib/homelab-manager/backup/jobs/`, résultats `restic --json` dans `.../runs/` ; aucun argument libre.
- Dépôts local, SFTP, S3/B2, rest-server ; copies `restic copy` ; secrets dans le coffre ; affichage unique du mot de passe du dépôt.
- Sources, exclusions glob vérifiées, rétention `forget`/`prune`, `check --read-data-subset` hebdomadaire.
- Instantanés, diff, historique des exécutions ; restauration simulée puis en quarantaine.
- Alerte en cas d'échec ou de retard (phase 4).
- Probablement découpée en 6A (dépôts, sauvegarde, historique) et 6B (rétention, vérification, restauration).

### 7 — Tâches planifiées en lecture

- Lecture de `/etc/crontab`, `/etc/cron.d/*`, `/etc/cron.{hourly,daily,weekly,monthly}` et timers (phase 3).
- Traduction française des expressions, prochaines exécutions dans le fuseau de la machine (analyseur `robfig/cron/v3` ou interne testé), masquage `password=` / `token=`.
- Dernières exécutions depuis les lignes `CRON[pid]` du journal (phase 2) ; code de sortie seulement pour les timers.
- Inspecteur d'expression sans enregistrement.

### 8 — WebAuthn et jetons d'API

- `github.com/go-webauthn/webauthn` ; configuration `webauthn_rp_id`, `webauthn_origins` ; HTTPS exigé hors `localhost`.
- Jetons `hlm_` + 32 octets, affichés une fois, hachés, portées en lecture seule, expiration 90 jours par défaut (sans expiration interdit), en-tête `Authorization: Bearer`. Le contrôle d'origine reste appliqué aux sessions par cookie, pas aux jetons.

## 5. Décisions à trancher

Ouvertes dans la spécification (`SPEC_ADMINISTRATION.md` § 10) : D-1 (édition Cron), D-2 (restauration en place), D-3 (pré-traitements de sauvegarde), D-4 (portées d'action des jetons), D-5 (actions et journaux Docker), D-6 (préférences système). Aucune ne bloque 1A–3. D-7 et D-8, ajoutées avec le module M, sont tranchées le 23 novembre 2026 : actions d'alimentation réservées à `admin`, sans arrêt programmé ni annulable.

Questions nouvelles relevées en préparant ce plan, à trancher avant l'incrément indiqué :

| Id | Question | Proposition | Avant |
|---|---|---|---|
| Q-1 | Un administrateur peut-il réinitialiser le mot de passe d'un autre compte ? La spécification ne le prévoit pas. | Non en 1A : supprimer puis recréer le compte. Ajouter une réinitialisation (nouveau mot de passe initial, à changer) en 1B si le besoin est confirmé. | 1B |
| Q-2 | Le compte `admin` créé avec le mot de passe par défaut `admin` doit-il être forcé à changer de mot de passe ? | Oui : `must_change_password = 1` lorsque `HOMELAB_ADMIN_PASSWORD` est vide. Non retenu en 1A pour ne pas changer le comportement documenté sans décision. | 1B |
| Q-3 | Les refus d'autorisation (`403 FORBIDDEN`) et les refus métier (`LAST_ADMIN`) sont-ils audités ? | Oui, avec `outcome = denied`, à partir de 1B (port `AuditRecorder`). En 1A, ils sont journalisés par zap avec le `request_id`. | 1B |
| Q-4 | Lever les limites v2.1 (pagination par curseur, images/volumes/réseaux inutilisés) ? La seconde exige d'ajouter au proxy des routes GET `images/json`, `volumes`, `networks`. | À planifier après la phase 3, si le besoin est confirmé. | Option |

## 6. Vérifications manuelles à chaque version

En plus de `scripts/verify.sh` (`TESTING.md`, parcours transverses) : installation sur Debian 13 avec les unités systemd, permissions des sockets, droits effectifs du compte `homelab-manager` (aucun root, groupes attendus seulement), thèmes clair et sombre, affichage mobile, navigation au clavier.
