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
| **3** | Services système en lecture (D-Bus), événements `service.failed` / `service.recovered` | 1A, D-Bus | `006_failed_units.sql` | M |
| **4** | Alertes et notifications : règles, cycle de vie, silences, canaux | 1C (secrets), 3 | `007_alerts.sql` (4A), `008_notifications.sql` (4B) | L |
| **5** | Actions sur les services par polkit (`start`, `stop`, `restart`, `reload`) | 1B (audit), 3 | — | M |
| **6** | Sauvegardes restic : dépôts, sources, exclusions, rétention, restauration en quarantaine | 1C, 4, 5 | `010_backups.sql` | XL |
| **7** | Tâches planifiées en lecture (Cron et timers) | 2, 3 | — | S |
| **8** | WebAuthn (passkeys, FIDO2) et jetons d'API en lecture — **livré** (8A, 8B) | 1C, HTTPS | `013_api_tokens.sql`, `014_webauthn_credentials.sql` | L |
| **V** | Vulnérabilités des paquets (Grype) : scans automatiques et manuels, onglet de la page Machine, synthèse du tableau de bord, historique | 4 | `009_vulnerabilities.sql` | L |
| *Option* | Dettes v2.1 : pagination par curseur, inventaire complet images/volumes/réseaux | Décision Q-4 | selon choix | M |

Le coffre de secrets (§ 2.5 de la spécification) est avancé en **1C** et non en phase 4 : les secrets TOTP doivent être chiffrés (§ 3.6), ce qui en fait le premier consommateur.

Ordre de réalisation : 1A → M → 1B → 1C → 2 → 3 → 4 → V → 5 → 6 → 7 → 8. Le module V, demandé le 24 novembre 2026, est placé juste après la phase 4 ; il ne dépend que d'elle (règle d'alerte). Il livre le démarrage d'une unité par D-Bus et polkit, que la phase 5 réutilise. Les phases 2 et 3 sont indépendantes de 1B et 1C et peuvent être avancées si le second facteur n'est pas prioritaire ; 7 peut suivre 2 et 3 sans attendre 4–6.

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

### 1C — Coffre de secrets et second facteur (livrée)

- Plan : [2026-11-23-phase-1c-second-facteur.md](./2026-11-23-phase-1c-second-facteur.md).
- Coffre : clé maître de 32 octets importée par systemd (`ImportCredential=homelab-manager-secrets-key`, chiffrée par `systemd-creds`) ou lue dans `secrets_key_file` ; AES-256-GCM, nonce aléatoire, UUID du compte en données associées ; mode dégradé explicite si la clé manque (TOTP indisponible, codes de secours et supervision intacts). Commande `systemd-creds encrypt` dans le README.
- TOTP interne (RFC 6238, SHA-1, 6 chiffres, 30 s, ±1 période, anti-rejeu par `last_used_step`) avec les vecteurs de la RFC ; QR code par `rsc.io/qr` v0.2.0, matrice dessinée en SVG par l'interface.
- 10 codes de secours affichés une fois, stockés en SHA-256, usage unique.
- Connexion en deux étapes : défi en mémoire de 5 minutes (cookie `homelab_login`), puis `POST /auth/login/second-factor` ; les codes faux comptent dans le verrouillage.
- Politique « second facteur obligatoire pour les administrateurs », activable seulement par un administrateur qui en a un ; les autres sont limités à l'enrôlement.
- Tables : `totp_secrets`, `recovery_codes` (migration `005`) ; pas encore de table `secrets` générique (phases 4 et 6).
- Parcours vérifié dans Chromium contre le binaire réel : enrôlement, QR code décodé, anti-rejeu, codes de secours, obligation pour un nouvel administrateur, mode dégradé sans clé.

### 2 — Journaux système (livrée)

- Plan : [2026-11-23-phase-2-journaux-systeme.md](./2026-11-23-phase-2-journaux-systeme.md).
- Constat du parcours réel : avec un curseur, `journalctl` refuse `--since` et ignore `--until` ; le Manager applique l'intervalle lui-même.
- Adaptateur `journalctl` : `exec` sans shell, arguments construits depuis des options validées (`--since`, `--until`, `--priority`, `--unit`, `--boot`, `--lines`, curseurs), sortie JSON bornée en taille et en lignes, délai 10 s.
- Recherche de sous-chaîne faite par le Manager (jamais une regex transmise), 500 entrées par page, pagination par curseur journald, suivi par interrogation toutes les 2 s (arrêt quand l'onglet est masqué).
- Masquage de `_CMDLINE` et des champs contenant `PASSWORD`, `TOKEN`, `SECRET`.
- Synthèse 24 h par tranches de 15 min, `--disk-usage`, export `.log` / `.json` (50 000 entrées au plus).
- Déploiement : ajouter `homelab-manager` au groupe `systemd-journal` (README, unité).
- Tests : construction des arguments, décodage JSON, bornes, masquage, et intégration contre le vrai `journalctl` sur des fichiers fabriqués par `systemd-journal-remote` (installé aussi en CI).

### 3 — Services système en lecture (livrée)

- Plan : [2026-11-23-phase-3-services-systeme.md](./2026-11-23-phase-3-services-systeme.md).
- Dépendance : `github.com/godbus/dbus/v5`, déjà présente (module M), au lieu de `go-systemd` prévu au § 2.4 de la spécification ; méthodes `ListUnits`, `GetUnit` et `Properties.GetAll` seulement.
- Collecte toutes les 30 s par une goroutine détenue par un usecase (arrêt par contexte, délai par cycle) ; inventaire services, timers, sockets avec états, PID, mémoire, CPU, dernier changement.
- Événements `service.failed` (sévérité `error`) et `service.recovered` dans la table `events` existante ; ensemble des unités en échec dans `failed_units` (migration `006`), pour ne signaler un échec qu'une fois.
- Détail d'unité : `FragmentPath`, `ExecStart` affiché tel quel, dépendances, limites, slice, 50 dernières lignes du journal (phase 2).
- Tests : faux systemd exporté par godbus sur un bus privé (`dbus-daemon`, installé en CI), transitions d'état, SQLite réel, routes `httptest`, page et fonctions pures de l'interface.
- Parcours vérifié dans le navigateur contre le Manager et un faux systemd sur un bus privé (macOS, sans vrai systemd) : inventaire, détail, événements d'échec puis de rétablissement, aucun doublon après redémarrage du Manager.

### 4 — Alertes et notifications (livrée)

- Plan : [2026-11-24-phase-4-alertes-notifications.md](./2026-11-24-phase-4-alertes-notifications.md), livré en deux parties : 4A (moteur, règles, cycle de vie, interface, sans communication sortante) puis 4B (canaux, silences, regroupement, renotification).
- Moteur évalué toutes les 15 s sur les derniers échantillons (aucune requête historique lourde) ; six types de règles (§ 6.1) ; règles par défaut désactivables.
- Cycle `firing` → `acknowledged` → `resolved`, silences (15 min à 7 jours), indicateurs.
- Canaux : Telegram, Discord, Gotify, ntfy, SMTP, webhook (signature HMAC-SHA256 optionnelle) ; secrets dans le coffre (1C) ; bouton « Tester » ; regroupement 1/min/canal ; renotification toutes les N minutes.
- Seule communication sortante du Manager : délais courts, échecs sans effet sur la supervision, file bornée.
- Tables : `alert_rules`, `alerts` (migration `007`), `alert_silences`, `notification_channels` (migration `008`).
- Règle « Sauvegarde » et règle par défaut « sauvegarde en retard » : avec la phase 6, qui en fournit les données.
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
- Livrée en trois incréments : 6A (dépôts, sauvegarde, historique), 6B (rétention, vérification, restauration) et 6C (diff, dépôt de copie) ; SFTP reste à décider.

### 7 — Tâches planifiées en lecture

- Lecture de `/etc/crontab`, `/etc/cron.d/*`, `/etc/cron.{hourly,daily,weekly,monthly}` et timers (phase 3).
- Traduction française des expressions, prochaines exécutions dans le fuseau de la machine (analyseur `robfig/cron/v3` ou interne testé), masquage `password=` / `token=`.
- Dernières exécutions depuis les lignes `CRON[pid]` du journal (phase 2) ; code de sortie seulement pour les timers.
- Inspecteur d'expression sans enregistrement.

### V — Vulnérabilités des paquets (Grype) (livré)

- Plan : [2026-11-24-module-v-vulnerabilites.md](./2026-11-24-module-v-vulnerabilites.md), livré en deux parties : VA (analyse, stockage, API, unité et règle polkit) puis VB (interface, tableau de bord, paramètres, règle d'alerte).

- Spécification : SPEC_ADMINISTRATION.md § 8 ter. Machine du Manager seule, sans agent (D-9) ; base de Grype mise à jour automatiquement chaque jour, désactivable (D-10) ; Grype exécuté par l'unité dédiée et durcie `homelab-vulnscan.service`, démarrée par D-Bus avec une règle polkit limitée à cette unité (D-11) ; décisions tranchées le 24 novembre 2026.
- Lecture des paquets en Go par gestionnaire (dpkg d'abord, puis RPM SQLite et apk), inventaire écrit pour l'unité, qui lance `grype sbom:` ; résultat JSON validé par le Manager ; un seul scan à la fois, délai de 15 min, ressources bornées par l'unité.
- Scans au premier démarrage puis périodiques (24 h par défaut, réglable), manuels (`operator`, audités) ; états, dernier et prochain scan, nouvelle tentative après un échec ; jamais d'ancien résultat présenté comme récent.
- Tables `vulnerability_scans` et `vulnerability_findings` (unicité, première et dernière détection, disparition), comparaison entre deux scans, rétention dédiée.
- Onglet « Vulnérabilités » de la page Machine (totaux, tableau filtrable et paginé, détail, lien vers la fiche publique, bouton « Analyser maintenant »), section du tableau de bord, type de règle d'alerte « vulnérabilités ».
- Installation de Grype documentée (paquet `.deb` officiel, somme de contrôle vérifiée, version figée) ; module indisponible sans lui.

### 8 — WebAuthn et jetons d'API

- `github.com/go-webauthn/webauthn` ; configuration `webauthn_rp_id`, `webauthn_origins` ; HTTPS exigé hors `localhost`. Livré en 8B comme second facteur (plan `2026-11-24-phase-8-webauthn-jetons.md`).
- Jetons `hlm_` + 32 octets, affichés une fois, hachés, portées en lecture seule, expiration 90 jours par défaut (sans expiration interdit), en-tête `Authorization: Bearer`. Le contrôle d'origine reste appliqué aux sessions par cookie, pas aux jetons.

## 5. Décisions à trancher

Ouvertes dans la spécification (`SPEC_ADMINISTRATION.md` § 10) : D-1 (édition Cron), D-2 (restauration en place), D-3 (pré-traitements de sauvegarde), D-4 (portées d'action des jetons), D-5 (actions et journaux Docker), D-6 (préférences système). Aucune ne bloque 1A–3. D-7 et D-8, ajoutées avec le module M, sont tranchées le 23 novembre 2026 : actions d'alimentation réservées à `admin`, sans arrêt programmé ni annulable.

Questions nouvelles relevées en préparant ce plan, à trancher avant l'incrément indiqué :

| Id | Question | Proposition | Avant |
|---|---|---|---|
| Q-1 | Un administrateur peut-il réinitialiser le mot de passe d'un autre compte ? | **Tranchée (oui)** le 23 novembre 2026 : nouveau mot de passe initial à changer, sessions révoquées, verrouillage levé ; livrée après 1B. | — |
| Q-2 | Le compte `admin` créé avec le mot de passe par défaut `admin` doit-il être forcé à changer de mot de passe ? | **Tranchée (oui)** le 23 novembre 2026 : changement imposé à la création, et au démarrage si le compte accepte encore `admin` ; livrée après 1B. | — |
| Q-3 | Les refus d'autorisation (`403 FORBIDDEN`) et les refus métier (`LAST_ADMIN`) sont-ils audités ? | Oui, avec `outcome = denied`, à partir de 1B (port `AuditRecorder`). En 1A, ils sont journalisés par zap avec le `request_id`. | 1B |
| Q-5 | Faut-il une procédure dans l'interface pour retirer le second facteur d'un compte qui a perdu son appareil et ses codes de secours ? | Non pour l'instant : un administrateur supprime et recrée le compte ; pour le dernier administrateur, procédure SQLite documentée dans le README (non auditée). À réexaminer avec WebAuthn. | 8 |
| Q-4 | Lever les limites v2.1 (pagination par curseur, images/volumes/réseaux inutilisés) ? La seconde exige d'ajouter au proxy des routes GET `images/json`, `volumes`, `networks`. | À planifier après la phase 3, si le besoin est confirmé. | Option |
| Q-6 | Règle par défaut « température > seuil du capteur » : les seuils des capteurs ne sont pas collectés | Seuil fixe de 85 °C, modifiable ; collecter les seuils serait une évolution à part | 4A |
| Q-7 | Un silence masque-t-il l'alerte dans l'interface ? | **Retenu (4B)** : non, alerte listée « sous silence », seules les notifications sont suspendues | — |
| Q-8 | Vérification réelle de Telegram, Discord, Gotify, ntfy et SMTP | Tests contre des serveurs simulés ; webhook signé vérifié en conditions réelles ; les autres restent à vérifier par le propriétaire avec ses jetons (bilan 4B) | Propriétaire |

## 6. Vérifications manuelles à chaque version

En plus de `scripts/verify.sh` (`TESTING.md`, parcours transverses) : installation sur Debian 13 avec les unités systemd, permissions des sockets, droits effectifs du compte `homelab-manager` (aucun root, groupes attendus seulement), thèmes clair et sombre, affichage mobile, navigation au clavier.
