# Spécification des modules d'administration — Homelab Manager

Version 3.0 (brouillon) — 22 novembre 2026 — **partiellement implémenté** : comptes multiples et rôles (§§ 3.1–3.2, phase 1A), sessions, politique de sécurité et journal d'audit (§§ 3.4–3.5, phase 1B), coffre de secrets, TOTP et codes de secours (§§ 2.5, 3.3 hors WebAuthn, phase 1C) journaux système (§ 4, phase 2), consultation des services système (§ 5.1, phase 3), vulnérabilités des paquets (§ 8 ter, module V) et redémarrage et extinction de la machine (§ 8 bis, module M), le 23 novembre 2026 ; règles, cycle de vie, indicateurs, silences et canaux des alertes (§§ 6.1 à 6.4, phase 4), actions sur les services système (§ 5.2, phase 5), sauvegardes restic (§ 7, phase 6 : incréments 6A et 6B) tâches planifiées en lecture (§ 8, phase 7) et jetons d'API (§ 3.5, phase 8A), le 24 novembre 2026 ; WebAuthn comme second facteur (§ 3.3, phase 8B), diff et copies des sauvegardes (§ 7, incrément 6C), et dépôts de sauvegarde SFTP (§ 7, incrément 6D), le 25 novembre 2026.

Ce document spécifie les modules annoncés « à venir » dans la navigation : utilisateurs et accès, journaux système, services système, alertes et notifications, sauvegardes, tâches planifiées, ainsi que le redémarrage et l'extinction de la machine (§ 8 bis, ajouté le 23 novembre 2026). Il s'appuie sur les maquettes de `maquette/` et en fixe le périmètre réel : une maquette montre une intention visuelle, ce document fait foi. Tant qu'un module n'est pas livré, il reste désactivé dans l'interface et ses routes n'existent pas.

Les décisions encore ouvertes sont signalées **[D-n]** et listées au § 10.

## 1. Règles intangibles

Elles priment sur toute maquette et sur toute demande future qui ne les révise pas explicitement.

1. **Aucun terminal, shell ni SSH** dans Homelab Manager, sous aucune forme : pas de console web, pas d'`exec` dans un conteneur, pas de session SSH, pas d'exécution de commande saisie par un utilisateur. Les boutons « Terminal / exec », « Clé SSH », « Tuer processus » et les sessions `pts/` des maquettes sont exclus.
2. **Les utilisateurs sont ceux de Homelab Manager uniquement.** Aucun compte Linux, groupe, PAM, `sudo`, `/etc/shadow` ou session SSH de la machine n'est lu ni modifié. Les maquettes qui affichent `uid`, groupes UNIX, « PAM / Linux Shadows » ou des politiques `pam_faillock` et `sudo` sont réinterprétées pour les seuls comptes du Manager (§ 3).
3. **Pas d'exécution de code arbitraire.** Aucune fonctionnalité ne permet de faire exécuter par la machine un programme, un script ou une ligne de commande fournis par l'utilisateur. Cela exclut la création ou l'édition de tâches Cron, d'unités systemd et de hooks de sauvegarde à commande libre.
4. **Le Manager ne tourne jamais en root.** Toute action privilégiée passe par le mécanisme du § 2.
5. **Légèreté.** Pas de Prometheus, Alertmanager, Grafana, Node Exporter, base externe ni broker : tout est intégré au binaire du Manager et à sa base SQLite. Les seuls programmes externes autorisés sont listés au § 2.3.
6. **Applications PaaS reportées.** Déploiements GitOps, applications, variables d'environnement, imports `.env`, dry-run et rollbacks (maquettes `applications_*`, `nouvelle_application_*`, `*_vaultwarden_server`, `configuration_git_*`) ne font pas partie de cette version. Ces fonctions restent assurées par Coolify. L'entrée de navigation « Applications » est retirée en attendant une version ultérieure.
7. **Pas d'action Docker mutatrice** dans ce lot (démarrer, arrêter, supprimer, `prune`) : le proxy Docker reste en lecture seule. Voir [D-5].

## 2. Modèle de privilèges

### 2.1 Principe

Le Manager s'exécute sous le compte `homelab-manager`. Chaque besoin privilégié est couvert par le mécanisme le plus étroit possible, jamais par un démon root généraliste :

| Besoin | Mécanisme | Portée accordée |
|---|---|---|
| Lire le journal systemd | Groupe `systemd-journal` | Lecture seule du journal |
| Lire l'état des unités et timers | D-Bus système, sans privilège | Lecture des propriétés publiques |
| Démarrer, arrêter, redémarrer, recharger des unités | D-Bus + règle **polkit** | Uniquement les unités de la liste blanche, uniquement ces 4 verbes |
| Exécuter une sauvegarde, un contrôle, une restauration | Unités systemd fournies `homelab-backup@.service`, démarrées via polkit | Uniquement ces instances, qui lisent leur configuration dans le répertoire d'état du Manager |
| Analyser les vulnérabilités des paquets (§ 8 ter) | Unité systemd fournie `homelab-vulnscan.service`, démarrée via polkit | Uniquement le verbe `start` sur cette unité, qui lit l'inventaire écrit par le Manager |
| Lire les crontabs système | Lecture de `/etc/crontab`, `/etc/cron.d`, `/etc/cron.*` (lisibles par tous) | Lecture seule |
| Redémarrer ou éteindre la machine (§ 8 bis) | D-Bus `systemd-logind` + règle **polkit** dédiée | Uniquement `org.freedesktop.login1.reboot`, `power-off` et leurs variantes `-multiple-sessions` ; jamais `-ignore-inhibit` ; inactif tant que `power_actions_enabled` est faux |

La règle polkit est installée par l'administrateur et générée à partir de la configuration (`/etc/polkit-1/rules.d/50-homelab-manager.rules`) par la sous-commande `homelab-manager polkit-rule`, qui lit la même configuration que le service (option retenue le 24 novembre 2026 par le propriétaire du projet : la règle ne peut pas diverger de la liste, que le Manager ne peut pas relire dans `/etc/polkit-1/rules.d/`). Elle n'accorde `org.freedesktop.systemd1.manage-units` qu'au compte `homelab-manager`, pour les verbes `start`, `stop`, `restart`, `reload` et pour les seules unités de la liste blanche `systemd_manageable_units`. Le Manager ne peut donc ni toucher une autre unité, ni lancer `daemon-reload`, ni activer ou désactiver une unité, même s'il était compromis.

### 2.2 Garanties communes à toute action

- **Confirmation explicite** dans l'interface pour toute action qui change l'état de la machine, avec rappel de la cible.
- **Rôle requis** vérifié côté serveur (§ 3.2) ; la validation de l'interface n'est jamais une protection.
- **Audit** : chaque action, réussie ou non, écrit une ligne `audit_logs` (acteur, cible, action, `requestId`, résultat) dans la transaction qui la déclenche.
- **Paramètres typés** : noms d'unités validés par motif (`^[A-Za-z0-9@._:-]{1,255}\.(service|timer|socket)$`) **et** liste blanche ; chemins absolus normalisés, sans `..` ; aucun paramètre libre transmis à un programme.
- **Délais et concurrence bornés** : une seule action par cible à la fois, délai maximal par action, refus `409 ACTION_IN_PROGRESS` sinon.
- **Limitation de débit** : 30 actions par minute et par utilisateur.

### 2.3 Programmes externes autorisés

| Programme | Usage | Règles |
|---|---|---|
| `journalctl` | Lecture du journal (§ 4) | `exec` sans shell, arguments construits par le code à partir d'options validées, sortie JSON bornée (taille, nombre de lignes), délai maximal 10 s |
| `restic` | Sauvegardes (§ 7) | Lancé uniquement par `homelab-backup@.service`, jamais par le Manager lui-même ; sortie `--json` |
| `grype` | Vulnérabilités des paquets (§ 8 ter) | Lancé uniquement par `homelab-vulnscan.service`, jamais par le Manager lui-même ; commandes fixes dans le fichier d'unité, sortie `-o json` bornée, 15 min au plus, une seule exécution à la fois |

Tout autre programme exige une révision de ce document.

### 2.4 Dépendances nouvelles prévues

| Dépendance | Usage | Justification |
|---|---|---|
| `github.com/coreos/go-systemd/v22/dbus` | Unités et timers via D-Bus | Go pur (godbus), évite d'exécuter `systemctl`. **Non retenue** : godbus seul suffit à la lecture (§ 5.1, phase 3) et aux quatre actions (§ 5.2, phase 5) |
| `github.com/godbus/dbus/v5` | Appels `systemd-logind` (§ 8 bis) | Client D-Bus Go pur, déjà dépendance de go-systemd ; le paquet `login1` de go-systemd ne renvoie pas l'erreur de `Reboot` / `PowerOff` |
| `github.com/go-webauthn/webauthn` | Passkeys et clés FIDO2 | Vérification d'attestation et d'assertion WebAuthn conforme ; à ne pas réimplémenter |
| `github.com/robfig/cron/v3` (analyseur seul) | Prochaines exécutions Cron | Go pur ; ou un analyseur interne équivalent, testé |
| Paquets Debian `restic`, `polkitd` | Sauvegardes, autorisations | Paquets officiels Debian 13 |

Le TOTP (RFC 6238) est implémenté en interne avec `crypto/hmac` (une cinquantaine de lignes, vecteurs de test RFC), sans dépendance.

### 2.5 Secrets

Les secrets que le Manager doit pouvoir réutiliser (jetons de notification, mots de passe de dépôts restic, identifiants S3) sont chiffrés en AES-256-GCM avec une clé maître. Cette clé est fournie par systemd via `LoadCredentialEncrypted=` (`systemd-creds`, liée au TPM 2.0 si présent, à défaut à la clé hôte). Mise en œuvre (phase 1C) : `ImportCredential=`, qui lit le même fichier chiffré dans `/etc/credstore.encrypted/` sans empêcher le démarrage s'il manque ; sans clé, le Manager démarre en mode dégradé explicite (TOTP indisponible, codes de secours utilisables). Premier usage : les secrets TOTP, avec l'UUID du compte en données associées. Les secrets ne sont jamais renvoyés par l'API après leur saisie : l'interface affiche seulement « défini » et l'empreinte SHA-256 tronquée. Les « scellements PCR[7] par variable » des maquettes ne sont pas retenus.

## 3. Utilisateurs et accès (module U) — maquettes `utilisateurs_acc_s_2fa_webauthn_*`, `connexion_s_curis_e_login_*`

Uniquement les comptes de Homelab Manager (règle 2).

### 3.1 Comptes

- Plusieurs comptes ; identifiant unique (`^[a-z0-9._-]{1,64}$`), nom affiché, rôle, état actif ou désactivé, date de dernière connexion et adresse IP source.
- Le compte initial `admin` reste le premier administrateur. Il est impossible de supprimer ou de rétrograder le dernier administrateur actif.
- Création par un administrateur, avec un mot de passe initial de 12 caractères au moins à changer à la première connexion. Suppression : révoque ses sessions et ses jetons, conserve ses lignes d'audit.
- **Réinitialisation par un administrateur** (décision Q-1, 23 novembre 2026) : un administrateur fixe un nouveau mot de passe initial (12 caractères au moins) pour un autre compte ; le titulaire doit le changer à sa connexion suivante, ses sessions sont révoquées et un verrouillage en cours est levé. Action auditée. Un administrateur change son propre mot de passe par la procédure normale, pas par une réinitialisation.
- **Mot de passe par défaut** (décision Q-2, 23 novembre 2026) : un compte `admin` créé avec le mot de passe intégré `admin` (aucun `HOMELAB_ADMIN_PASSWORD`) doit changer de mot de passe à sa première connexion. Au démarrage, si le compte existant accepte encore ce mot de passe par défaut, le même changement est imposé.

### 3.2 Rôles

| Rôle | Consultation | Actions services et sauvegardes | Alertes (acquitter, silences) | Paramètres, règles, canaux | Comptes et jetons |
|---|---|---|---|---|---|
| `viewer` | oui | non | non | non | son profil seulement |
| `operator` | oui | oui | oui | non | son profil seulement |
| `admin` | oui | oui | oui | oui | oui |

« Son profil » : mot de passe, facteurs 2FA, ses sessions, ses jetons.

### 3.3 Second facteur

- **TOTP** (RFC 6238, SHA-1, 6 chiffres, période 30 s, tolérance ±1 période, code non réutilisable dans sa fenêtre) avec enrôlement par QR code et vérification d'un premier code.
- **WebAuthn** (passkeys et clés FIDO2) : plusieurs clés par compte, nom libre, date d'enrôlement et de dernier usage ; vérification de l'utilisateur requise. L'identifiant de partie dépendante (`RP ID`) est le nom d'hôte de l'interface et doit être configuré (`webauthn_rp_id`, `webauthn_origins`) ; WebAuthn exige HTTPS sauf sur `localhost`.
- **Codes de secours** : 10 codes à usage unique, affichés une seule fois, stockés hachés.
- La connexion se fait en deux étapes : mot de passe, puis second facteur si le compte en a un. Si la politique l'autorise (décision D-14, désactivé par défaut), un compte qui a une passkey peut aussi se connecter avec elle seule. Le bouton « Clé SSH » de la maquette est exclu.
- *Livré en phase 8B* : jusqu'à 10 clés par compte, attestation non demandée (`none`), vérification de l'utilisateur exigée, refus d'un compteur de signatures qui recule ; la première clé d'un compte sans second facteur lui donne des codes de secours. La connexion par passkey seule, sans mot de passe, est livrée le 25 novembre 2026 selon D-14.

### 3.4 Politique de sécurité (réglable par un administrateur)

- Second facteur obligatoire pour les administrateurs (par défaut : désactivé tant qu'aucun facteur n'est enrôlé, pour ne pas bloquer l'installation).
- Expiration d'une session après N minutes d'inactivité (défaut 60) en plus de la durée maximale de 12 h.
- Verrouillage d'un compte après 5 échecs consécutifs pendant 15 minutes (s'ajoute à la limite par IP existante).
- Liste de réseaux autorisés (CIDR) pour l'interface ; vide = tous. Une modification qui exclurait l'adresse de l'administrateur en cours est refusée.

### 3.5 Sessions et jetons

- Liste des sessions actives de l'utilisateur (navigateur, IP, méthode d'authentification, dernière activité), révocation unitaire et « toutes sauf celle-ci ». Un administrateur voit et révoque les sessions de tous.
- **Jetons d'API** : préfixe `hlm_`, 32 octets aléatoires, affichés une seule fois, stockés hachés ; nom, portées, date d'expiration (90 jours par défaut, « sans expiration » interdit) et dernier usage. En-tête `Authorization: Bearer`. Portées v3 : `metrics:read`, `containers:read`, `events:read`, `services:read`, `backups:read`. Aucune portée d'action pour les jetons dans cette version [D-4]. **Mise en œuvre (phase 8A, 24 novembre 2026)** : jetons `hlm_` + 32 octets en base64url, stockés en SHA-256 (entropie suffisante, pas de hachage lent), préfixe de 12 caractères affiché ; validité de 1 à 365 jours ; 20 jetons par compte ; chacun gère les siens, un administrateur liste et révoque ceux de tous ; un compte désactivé ou supprimé invalide ses jetons ; création et révocation auditées. Portées appliquées aux seules routes `GET` qu'elles nomment, tout le reste refusé. Carte « Jetons d'API » des Paramètres. Contrat : API_CONTRACTS.md § 3.6.
- **Journal d'audit** consultable par les administrateurs, filtrable, exportable en CSV et JSON.

### 3.6 Données

Nouvelles tables : `user_roles` (ou colonne `role` sur `users`), `totp_secrets` (secret chiffré), `webauthn_credentials`, `recovery_codes`, `api_tokens`, `security_policy` (singleton), `login_failures`. `sessions` reçoit `last_seen_at` et `auth_method`.

## 4. Journaux système (module J) — maquette `journaux_syst_me_*`

Lecture seule du journal systemd.

- **Sources** : tout le journal, ou filtré par unité, noyau (`_TRANSPORT=kernel`), authentification (`SYSLOG_FACILITY=4,10`), démon Docker (`_SYSTEMD_UNIT=docker.service`).
- **Filtres** : priorité minimale (`emerg` … `debug`, défaut `info`), unité (liste issue du module S), démarrage (actuel ou précédents, `--list-boots`), intervalle `[from, to)`, texte recherché. La recherche textuelle est une recherche de sous-chaîne insensible à la casse faite par le Manager, pas une expression régulière transmise à `journalctl`.
- **Lecture** : `journalctl -o json --no-pager` avec `--since`, `--until`, `--priority`, `--unit`, `--boot`, `--lines` construits à partir des filtres validés ; 500 entrées par page au plus ; pagination par curseur journald (`--after-cursor` / `--before-cursor`).
- **Suivi en direct** : interrogation périodique (2 s) avec le dernier curseur ; pas de WebSocket. Le suivi s'arrête quand l'onglet est masqué.
- **Inspecteur** : champs de l'entrée sélectionnée (`PRIORITY`, `_SYSTEMD_UNIT`, `_PID`, `_COMM`, `_EXE`, `_BOOT_ID`, `__REALTIME_TIMESTAMP`, `MESSAGE`, …) et JSON brut. Les champs `_CMDLINE` et ceux contenant `PASSWORD`, `TOKEN`, `SECRET` sont masqués.
- **Synthèse** : volume par priorité sur 24 h par tranches de 15 min (requête dédiée, bornée), occupation disque du journal (`journalctl --disk-usage`).
- **Export** : `.log` (format court) ou `.json` du résultat filtré, 50 000 entrées au plus.
- Non retenus : scellement FSS et compression affichés par la maquette (informations non fiables à obtenir sans privilège).
- Mise en œuvre (phase 2) : la liste des unités vient du journal lui-même (`journalctl --field=_SYSTEMD_UNIT`) ; elle reste utilisée après la phase 3, car le journal cite aussi des unités d'autres types ou déchargées, absentes de l'inventaire du module S. Avec un curseur, `journalctl` refuse `--since` et ignore `--until` : le Manager applique alors l'intervalle lui-même. Contrat exact : API_CONTRACTS.md § 4.6.

## 5. Services système (module S) — maquette `services_syst_me_*`

### 5.1 Consultation

- Inventaire des unités chargées (services, timers, sockets) via D-Bus : nom, description, état de chargement (`loaded`, `not-found`…), état de démarrage (`enabled`, `static`…), état actif et sous-état, PID principal, mémoire (`MemoryCurrent`), temps CPU (`CPUUsageNSec`), date du dernier changement d'état, résultat de la dernière exécution.
- Compteurs : unités actives, en échec, timers actifs avec prochaine et dernière exécution.
- Détail d'une unité, en lecture : fichier de définition (`FragmentPath`), `ExecStart` affiché tel quel, `Requires`, `After`, politique `Restart`, limites (`TasksMax`, `LimitNOFILE`, `MemoryMax`), slice cgroup, extrait des 50 dernières lignes du journal (module J).
- Collecte toutes les 30 s ; une unité qui passe en `failed` crée un événement `service.failed` (sévérité `error`) et, au retour en `active`, `service.recovered`.
- Filtres : type, état, recherche par nom ; tri « en échec d'abord ».
- Mise en œuvre (phase 3) : `github.com/godbus/dbus/v5` (déjà utilisé par le module M) au lieu de `go-systemd`, dont seules ces lectures seraient utilisées (écart au § 2.4, sans effet sur les garanties) ; méthodes `ListUnits`, `GetUnit` et `Properties.GetAll` seulement. 2 000 unités au plus, en échec d'abord. Les unités enregistrées en échec sont gardées en base (`failed_units`) pour qu'un échec ne soit signalé qu'une fois. Un nom d'unité échappé par systemd (contenant `\`) est hors du motif du § 2.2 : l'unité est listée mais son détail est refusé. Contrat exact : API_CONTRACTS.md § 4.7.

### 5.2 Actions

- `start`, `stop`, `restart`, `reload` sur les unités de la liste blanche `systemd_manageable_units` (configuration du service, vide par défaut) ; « Exécuter » un timer = `start` du service qu'il déclenche, si ce service est dans la liste blanche.
- Rôle `operator` au moins ; confirmation ; audit ; résultat suivi jusqu'à l'état stable (délai maximal 90 s).
- Exclus : créer ou éditer une unité, `daemon-reload`, `enable`/`disable`, `mask`, tuer un processus, ouvrir un « cgroup viewer » (règle 3).
- Mise en œuvre (phase 5, 24 novembre 2026) : liste blanche `systemd_manageable_units` du fichier de configuration (ou `HOMELAB_SYSTEMD_MANAGEABLE_UNITS`), validée au démarrage (motif du § 2.2, modèles `nom@.service` refusés, 200 noms au plus) ; règle polkit générée par `homelab-manager polkit-rule` (§ 2.1). Le Manager vérifie la liste avant tout appel ; polkit la vérifie de nouveau. Appels `StartUnit`, `StopUnit`, `RestartUnit`, `ReloadUnit` en mode `replace` par godbus, puis lecture du job et de l'état jusqu'à un état stable. Une action par unité, 30 par minute et par utilisateur ; audit `requested` avant l'appel puis `success` ou `failure`. « Exécuter » un timer est proposé par l'interface comme un `start` du service déclenché, qui doit lui-même être dans la liste. L'état de la dernière action par unité est gardé en mémoire, sans table. Plan et bilan : `docs/superpowers/plans/2026-11-24-phase-5-actions-services.md` ; contrat : API_CONTRACTS.md § 4.7.

## 6. Alertes et notifications (module A) — maquette `alertes_notifications_*`

Moteur intégré au Manager (règle 5), évalué sur les données qu'il collecte déjà.

**Mise en œuvre (phase 4A, 24 novembre 2026)** : règles des types métrique hôte, métrique conteneur, état de conteneur, unité en échec et collecteur (la règle « Sauvegarde » et sa règle par défaut sont livrées avec l'incrément 6A du module B) ; cycle `firing` → `acknowledged` → `resolved` et indicateurs ; liens vers l'unité, son journal et le conteneur. La règle par défaut de température utilise un seuil fixe de 85 °C, modifiable, les seuils des capteurs n'étant pas collectés. **Phase 4B (24 novembre 2026)** : silences par règle, par cible (une règle sur une cible) ou globaux, l'alerte restant listée « sous silence » (décision Q-7) ; six canaux, secrets scellés par le coffre et jamais renvoyés, routage par sévérité, regroupement à un message par minute et par canal, renotification d'une alerte `firing` non acquittée, bouton « Tester », dernier envoi et dernière erreur. L'action « Redémarrer » est livrée avec la phase 5 (§ 6.4). Contrat : API_CONTRACTS.md § 4.8.

### 6.1 Règles

| Type | Paramètres | Exemple de la maquette |
|---|---|---|
| Seuil de métrique hôte | métrique, labels, opérateur (`>`, `>=`, `<`, `<=`), seuil, durée continue | CPU > 90 % pendant 5 min ; `filesystem.percent{mountpoint=/var/lib/docker}` > 85 % |
| Seuil de métrique conteneur | conteneur (ou tous), métrique, opérateur, seuil, durée | mémoire > 90 % de la limite |
| État de conteneur | conteneur (ou tous), états déclencheurs, durée | conteneur `exited` ou en redémarrage en boucle |
| Unité systemd en échec | unité ou motif | `watchtower.service` en `failed` |
| Collecteur indisponible | système ou Docker, durée | Docker `unavailable` depuis 2 min |
| Sauvegarde | dernier succès plus ancien que N heures, ou échec | aucune sauvegarde réussie depuis 26 h |

- Évaluation toutes les 15 s sur les derniers échantillons, sans requête historique lourde. Sévérités `info`, `warning`, `critical`. Règles fournies par défaut (désactivables) : disque > 85 %, mémoire disponible < 10 %, température > seuil du capteur, collecteur indisponible, unité en échec, sauvegarde en retard.
- Non retenus : PromQL, « SMART reallocated sectors » et expiration de certificats (données non collectées aujourd'hui ; à ajouter avec une collecte dédiée si besoin).

### 6.2 Cycle de vie

`firing` → (`acknowledged`) → `resolved`, avec historique. Une alerte acquittée reste ouverte jusqu'à sa résolution, mais ne renotifie plus. **Silence** : par règle, par cible ou global, durée de 15 min à 7 jours, avec auteur et motif. Indicateurs : alertes ouvertes par sévérité, résolues sur 24 h, durée moyenne de résolution.

### 6.3 Canaux

- Telegram (bot + chat), Discord (webhook), Gotify, ntfy, e-mail SMTP (STARTTLS ou TLS), webhook HTTP générique (JSON documenté, signature HMAC-SHA256 optionnelle).
- Secrets chiffrés (§ 2.5) ; bouton « Tester » ; état du dernier envoi et dernière erreur ; routage par sévérité ; renotification d'une alerte non acquittée toutes les N minutes (défaut 30) ; regroupement des notifications à 1 par minute et par canal.
- Les envois sortants sont, avec la mise à jour de la base de Grype (§ 8 ter.2), la seule communication réseau du Manager vers l'extérieur ; ils sont optionnels et n'affectent pas la supervision s'ils échouent.

### 6.4 Actions depuis une alerte

Liens vers le journal de l'unité (module J) et action « Redémarrer » si l'unité est dans la liste blanche (module S). « Nettoyer Docker (prune) » et « Augmenter le seuil » en un clic ne sont pas retenus ; le seuil se modifie dans la règle.

Mise en œuvre (phase 5, 24 novembre 2026) : « Redémarrer » est proposé aux `operator` et `admin` sur une alerte ouverte de type « unité en échec » dont l'unité est dans la liste blanche, avec la même confirmation, le même audit et le même suivi que la page Services.

## 7. Sauvegardes et restauration (module B) — maquettes `sauvegardes_*`, `param_tres_sauvegardes_*`, `r_gles_d_exclusion_*`, `journaux_de_sauvegarde_*`

Le Manager configure et pilote **restic** ; il ne réimplémente ni le chiffrement ni la déduplication.

**Mise en œuvre (incrément 6A, 24 novembre 2026)** : dépôt local ou monté, rest-server et S3 (dont B2), initialisation avec mot de passe généré et affiché une fois, sources, exclusions, sauvegardes manuelles (`operator`) et planifiées par le Manager (D-12), historique des exécutions, liste des instantanés, page « Sauvegardes ». Écarts : le « script d'entrée » est un binaire Go, `homelab-backup-runner`, qui valide de nouveau le fichier de travail et lance restic sans shell (sources après `--`, exclusions par fichier, secrets par l'environnement de restic seul) ; les fichiers échangés sont dans `/var/lib/homelab-backup/jobs/` (Manager, 0700) et `runs/` (root, lisible par le groupe du Manager), comme pour le module V ; le mot de passe et les clés passent par un fichier à usage unique que le programme supprime après lecture ; l'unité, root, n'écrit que son cache, ses résultats, ses fichiers de travail et les points de montage usuels (`/mnt`, `/media`, `/srv`, `/var/backups`), un dépôt local ailleurs exigeant un complément d'unité. **Incrément 6B (24 novembre 2026)** : rétention `forget --prune` et vérification `check --read-data-subset` réglables, enchaînées après une sauvegarde planifiée réussie (la vérification au plus une fois par semaine) et lançables à la main ; restauration simulée puis réelle dans `/var/lib/homelab-backup/restore/<exécution>/` (0700, root) ; rattachement d'un dépôt existant par son mot de passe ; déverrouillage forcé avec avertissement ; opérations réservées aux administrateurs. Non retenus dans cet incrément : diff entre instantanés, copies vers un autre dépôt (`restic copy`), SFTP (gestion de la clé SSH de root à décider). **Incrément 6C (25 novembre 2026)** : diff entre deux instantanés (`restic diff`, opération `diff`, administrateurs) avec ses chiffres complets et ses 1 000 premiers chemins, servis aux seuls administrateurs ; **un** dépôt de copie (`restic copy`, opération `copy`) avec son propre mot de passe, créé avec `--copy-chunker-params` ou rattaché, copié à la main ou après chaque sauvegarde planifiée réussie (avant la rétention), la politique de rétention s'appliquant aussi à lui. Écarts : un seul dépôt de copie au lieu de « des copies » (§ 7.2) ; deux dépôts S3 doivent partager leur clé d'accès, restic 0.18 n'en lisant qu'une pour une copie. **Incrément 6D (25 novembre 2026)** : dépôts SFTP, principal ou de copie, `sftp://utilisateur@hôte[:port]/chemin/absolu`, selon la décision D-13 : une clé SSH Ed25519 propre au Manager, générée et remplaçable par un administrateur, partie privée scellée dans le coffre et transmise par le fichier de secrets à usage unique, partie publique affichée pour être installée sur les serveurs ; clé d'hôte de chaque serveur épinglée par l'administrateur (contenu de `ssh_host_*_key.pub` ou ligne de `ssh-keyscan`), aucune clé acceptée à la première connexion ; ssh lancé par restic avec `-F /dev/null` et des options fixes, sans agent, mot de passe, redirection ni proxy. Le client `ssh` (paquet `openssh-client`) doit être installé ; l'unité n'a pas changé. La règle d'alerte « Sauvegarde » (dernier succès plus ancien que N heures, 1 à 720, ou dernière sauvegarde en échec ; gelée sans dépôt initialisé) et sa règle par défaut de 26 h sont livrées avec 6A (migration `011`). Plan : `docs/superpowers/plans/2026-11-24-phase-6-sauvegardes.md` ; contrat : API_CONTRACTS.md § 4.10.

### 7.1 Exécution

- Les opérations restic s'exécutent dans `homelab-backup@<opération>.service`, unités root fournies par le projet et durcies (`ProtectSystem=strict`, `ProtectHome=read-only`, `ReadWritePaths=` limité au cache restic et au répertoire de restauration, `NoNewPrivileges`, `PrivateTmp`, réseau autorisé pour les dépôts distants). Opérations : `backup`, `check`, `forget-prune`, `restore`, `snapshots`, `diff`, `init`.
- L'unité lit un fichier de travail JSON écrit par le Manager dans `/var/lib/homelab-manager/backup/jobs/` (opération, dépôt, chemins, options validées) et écrit sa progression et son résultat (`restic --json`) dans `/var/lib/homelab-manager/backup/runs/`. Le Manager ne passe aucun argument libre : le script d'entrée de l'unité traduit le fichier de travail en arguments restic selon un schéma fixe.
- Planification réglée dans l'interface (quotidienne ou hebdomadaire, heure de la machine) et tenue par le Manager, qui démarre lui-même l'unité ; une sauvegarde manquée pendant un arrêt est lancée une fois au redémarrage (décision D-12). Le timer `homelab-backup-schedule.timer` initialement prévu n'est pas retenu.

### 7.2 Dépôts

- Types : répertoire local ou monté (y compris un partage SMB/NFS déjà monté par le système), SFTP, S3 compatible (dont Backblaze B2), rest-server. Le montage SMB lui-même n'est pas géré par le Manager.
- Un dépôt principal et des copies (`restic copy`) vers d'autres dépôts, pour la règle 3-2-1. La synchronisation rclone de la maquette n'est pas retenue.
- Mot de passe du dépôt et identifiants chiffrés (§ 2.5) ; affichage unique du mot de passe à l'initialisation, avec la consigne d'en conserver une copie hors ligne (la « clé papier BIP39 » de la maquette n'est pas retenue).
- Test de connexion, espace utilisé (`restic stats`), état du verrou ; « Forcer le déverrouillage » réservé aux administrateurs, avec avertissement.

### 7.3 Sources, exclusions, rétention

- Sources : chemins absolus choisis par l'administrateur (ex. `/var/lib/docker/volumes`, `/etc`, `/opt/stacks`), étiquettes par source.
- Exclusions : liste de motifs glob restic éditable (tableau ou texte brut), syntaxe vérifiée, `--exclude-caches` et `--exclude-if-present` en options ; « gain estimé » calculé au dernier run à partir des fichiers exclus.
- **Pas de hooks à commande libre** (règle 3). Pré-traitements intégrés envisagés : export SQLite cohérent (`.backup`) d'une liste de fichiers de base [D-3].
- Rétention `forget --keep-daily/weekly/monthly/yearly` puis `prune`, planifiés ; `check --read-data-subset` périodique (défaut 5 % chaque semaine).

### 7.4 Consultation et restauration

- Instantanés : identifiant, date, dépôt, chemins, étiquettes, taille ajoutée et totale, nombre de fichiers ; filtres ; diff entre deux instantanés (`restic diff`).
- Historique des exécutions avec étapes, durée, débit, code de sortie et journal ; alerte en cas d'échec (module A).
- Restauration : simulation (`--dry-run`) d'abord, puis restauration dans un **répertoire de quarantaine** `/var/lib/homelab-manager/restore/<id>/` (défaut) ou dans un répertoire vide choisi par l'administrateur. La restauration « à l'emplacement d'origine » et le téléchargement `.tar.gz` ne sont pas retenus dans ce lot [D-2]. Le montage FUSE d'un instantané n'est pas retenu.

## 8. Tâches planifiées (module C) — maquette `t_ches_cron_*`

**Consultation uniquement** dans cette version : créer, modifier, suspendre ou lancer une tâche Cron revient à exécuter du code arbitraire en root (règle 3) [D-1].

**Mise en œuvre (phase 7, 24 novembre 2026)** : analyseur d'expressions interne, testé, sans dépendance (`robfig/cron` non retenu) ; inventaire de `/etc/crontab`, `/etc/cron.d` et des scripts run-parts avec la ligne qui les lance et la mention d'anacron ; timers de l'inventaire des unités avec le résultat du service déclenché ; dernières exécutions lues dans les lignes `CRON` du journal (par l'identifiant : les tâches tournent dans une session PAM, hors de `cron.service`) ; masquage étendu aux variables et options nommées d'après un secret, à l'en-tête `Authorization`, à `-u utilisateur:motdepasse`, à `-pMOTDEPASSE` et aux identifiants d'URL ; inspecteur sans enregistrement. Page « Tâches planifiées ». Contrat : API_CONTRACTS.md § 4.11.

- Inventaire : `/etc/crontab`, `/etc/cron.d/*`, scripts de `/etc/cron.hourly|daily|weekly|monthly`, et timers systemd (module S). Les crontabs utilisateurs (`/var/spool/cron/crontabs`) ne sont pas lisibles sans privilège et ne sont pas affichées.
- Pour chaque entrée : expression, traduction en français (« tous les jours à 03:00 »), utilisateur d'exécution, commande affichée telle quelle (masquage des motifs `password=`, `token=`), fichier source, prochaines exécutions calculées dans le fuseau de la machine.
- Dernières exécutions : lignes `CRON[pid]` du journal (module J). Cron ne journalise pas le code de sortie : il n'est affiché que pour les timers systemd. Le « taux de succès » de la maquette n'est donc pas calculable pour Cron et n'est pas retenu.
- Inspecteur d'expression : saisie d'une expression et affichage des 5 prochaines exécutions, sans enregistrement.

## 8 bis. Alimentation de la machine (module M)

Ajouté le 23 novembre 2026 à la demande du propriétaire du projet : redémarrer ou éteindre la machine depuis la page Machine.

- **Actions** : `reboot` (Redémarrer) et `poweroff` (Éteindre), demandées à `systemd-logind` par D-Bus (`org.freedesktop.login1.Manager.Reboot` / `PowerOff`, sans authentification interactive). Aucune commande (`shutdown`, `systemctl`, `reboot`) n'est exécutée.
- **Double activation, désactivée par défaut** : le réglage de service `power_actions_enabled` (`HOMELAB_POWER_ACTIONS_ENABLED`, défaut `false`) **et** la règle polkit fournie `deployments/polkit/50-homelab-manager-power.rules`, installée par l'administrateur. Sans l'un ou l'autre, aucune action n'aboutit.
- **Rôle** `admin` uniquement (décision D-7, tranchée le 23 novembre 2026).
- **Confirmation** : saisir le nom exact de la machine, vérifié par le serveur contre le nom d'hôte courant ; avertissement explicite des conséquences (services et conteneurs arrêtés ; après extinction, Homelab Manager ne peut pas rallumer la machine).
- **Inhibitions respectées** : les actions `-ignore-inhibit` ne sont pas accordées ; une sauvegarde ou un autre programme qui tient un verrou d'inhibition bloque la demande, refusée avec un message explicite.
- **Audit avant l'action** : une ligne (`host`, nom d'hôte, `reboot` ou `poweroff`, `requested`) est écrite avant la demande à logind ; sans écriture possible, rien n'est demandé. Un refus ajoute une ligne `failure`.
- **Une seule demande** : après une demande acceptée, les suivantes sont refusées (`409 ACTION_IN_PROGRESS`) jusqu'à l'arrêt du Manager.
- **Arrêt propre** : logind arrête les services, dont le Manager, qui attend ses collecteurs puis ferme la base ; Docker arrête les conteneurs selon sa configuration. Au redémarrage, systemd relance le Manager ; l'interface signale l'indisponibilité pendant l'intervalle.
- **Exclus** : arrêt programmé ou annulable (décision D-8, tranchée le 23 novembre 2026), veille, hibernation, redémarrage vers le firmware, `kexec`, réveil par le réseau (Wake-on-LAN).

## 8 ter. Vulnérabilités des paquets (module V)

Ajouté le 24 novembre 2026 à la demande du propriétaire du projet : détecter les vulnérabilités connues (CVE et avis des distributions) des paquets système installés avec [Grype](https://github.com/anchore/grype) (Anchore, licence Apache 2.0), les consulter et en voir la synthèse sur le tableau de bord. D-9 (machine du Manager seule), D-10 (mise à jour automatique de la base) et D-11 (unité systemd dédiée) sont tranchées.

**Mise en œuvre (module V, 24 novembre 2026)** : livré pour Debian, Ubuntu et leurs dérivées (dpkg) et Alpine (apk) ; **lecteur RPM (25 novembre 2026)** : Fedora, RHEL, Rocky Linux, AlmaLinux et dérivées par `/var/lib/rpm/rpmdb.sqlite` (rpm 4.16 et suivants), ouverte en lecture seule par le pilote SQLite du projet, en-têtes RPM décodés par le Manager (nom, version, release, epoch, architecture, RPM source, label de modularité), clés `gpg-pubkey` ignorées ; une base Berkeley DB (RHEL 8 et dérivées) est signalée « non prise en charge ». Écarts : les fichiers échangés avec l'unité sont dans `/var/lib/homelab-vulnscan/inventory/` et `result/` (répertoires `tmpfiles.d` en 2750, chacun lisible par le groupe de l'autre compte) et non dans le répertoire d'état du Manager, en 0700 ; la règle d'alerte prend une sévérité minimale et l'option « corrigeables seulement », sans règle par défaut. Grype figé à la version 0.121.3. Plan et bilan : `docs/superpowers/plans/2026-11-24-module-v-vulnerabilites.md` ; contrat : API_CONTRACTS.md § 4.9.

### 8 ter.1 Périmètre

- **Machine** : uniquement celle du Manager, sans agent ni hôte distant (décision D-9, tranchée le 24 novembre 2026). Les scans sont faits par le Manager lui-même.
- **Distributions** : détection par `/etc/os-release` (`ID`, `VERSION_ID`, `ID_LIKE`). Debian 13, cible de déploiement, d'abord ; Ubuntu (même format de paquets), puis Fedora, Rocky Linux, AlmaLinux (base RPM SQLite) et Alpine (`/lib/apk/db/installed`). Un lecteur par gestionnaire de paquets derrière un port unique : ajouter une distribution = ajouter un lecteur et ses tests. Une distribution non reconnue est signalée comme telle, sans scan.
- **Paquets** : lus par le Manager en Go, sans privilège (fichiers de base de paquets lisibles par tous), puis transmis à Grype sous forme d'inventaire (SBOM au format JSON de Syft, `grype sbom:<fichier>`, § 8 ter.2). Grype ne parcourt donc pas le système de fichiers : scan rapide, sans accès root, limité aux paquets système. Les dépendances embarquées dans des binaires ou des images Docker sont hors périmètre de ce module.
- **Résultat présenté comme une vulnérabilité identifiée**, jamais comme une exploitabilité mesurée : une vulnérabilité d'un paquet installé peut ne pas être atteignable dans la configuration de la machine.

### 8 ter.2 Exécution de Grype

- **Unité dédiée** (décision D-11) : Grype n'est jamais lancé par le Manager lui-même, seulement par l'unité fournie `homelab-vulnscan.service` (`Type=oneshot`), sur le modèle de `homelab-backup@.service` (§ 7.1). Déroulement d'un scan :
  1. le Manager lit `/etc/os-release` et la base de paquets, puis écrit l'inventaire (SBOM) dans `/var/lib/homelab-manager/vulnscan/inventory.json` ;
  2. il démarre l'unité par D-Bus (`StartUnit`), autorisé par une règle polkit qui n'accorde au compte `homelab-manager` que le verbe `start` sur cette seule unité ;
  3. l'unité met à jour la base si c'est permis (D-10), lance `grype sbom:…/inventory.json -o json --file …/result.json` et se termine ; ses commandes sont écrites dans le fichier d'unité, aucune donnée du Manager ni d'un utilisateur ne s'y ajoute ;
  4. le Manager suit l'état de l'unité par D-Bus jusqu'à sa fin (15 min au plus, `TimeoutStartSec=`), puis lit `result.json`, borné en taille et décodé strictement.
- **Durcissement de l'unité** : compte système dédié `homelab-vulnscan`, sans shell ; `CPUWeight=idle`, `IOSchedulingClass=idle`, `MemoryMax=`, `Nice=19` ; `ProtectSystem=strict`, `ProtectHome=yes`, `PrivateTmp=yes`, `NoNewPrivileges=yes`, `CapabilityBoundingSet=` vide ; écriture permise seulement dans son répertoire de base (`/var/cache/homelab-vulnscan`) et dans le fichier de résultat ; réseau limité au téléchargement de la base (aucune écoute). Une seule exécution à la fois : systemd refuse de démarrer une unité déjà active, et le Manager tient aussi son propre verrou.
- Le Manager n'a donc ni à lancer `grype`, ni à accéder au réseau pour ce module : il produit l'inventaire, démarre l'unité et lit le résultat. Le mécanisme D-Bus et polkit de démarrage d'une unité est livré avec ce module et réutilisé par la phase 5.
- **Base de vulnérabilités** (décision D-10) : mise à jour automatique une fois par jour depuis l'infrastructure d'Anchore (environ 100 Mo, HTTPS, certificat vérifié), avant le scan, par l'unité dédiée et non par le Manager ; c'est, avec les notifications (§ 6.3), la seule communication sortante liée au Manager. Désactivable dans la configuration du service ; la base s'importe alors à la main (`grype db import`). Une mise à jour en échec n'empêche pas le scan tant que la base a moins de 5 jours (réglage de Grype) ; au-delà, le scan échoue (« base périmée ») au lieu de produire un résultat trompeur. La date de construction de la base est affichée avec chaque scan.
- **Installation** : Grype n'est pas dans les dépôts Debian ; paquet `.deb` officiel des releases GitHub d'Anchore, somme de contrôle (et signature cosign) vérifiée, version figée publiée depuis au moins deux semaines, documentée dans le README. Sans Grype, le module est « indisponible » et le reste du Manager fonctionne.

### 8 ter.3 Scans

- Automatiques : un premier scan au démarrage du Manager s'il n'en existe aucun, puis périodiques (défaut 24 h, réglable de 6 h à 7 jours dans les Paramètres) ; manuel depuis l'interface (rôle `operator` au moins, audité).
- États : `pending`, `running`, `succeeded`, `failed` (avec la cause : Grype absent, base injoignable ou périmée, délai dépassé, sortie invalide), date du dernier scan réussi et du prochain scan prévu. Un scan demandé pendant un autre est refusé (`409 SCAN_IN_PROGRESS`) ; un échec est retenté une fois après 15 min, puis au cycle suivant.
- Un échec n'efface pas le dernier résultat réussi, mais l'interface affiche clairement sa date et l'échec du scan suivant : jamais d'ancien résultat présenté comme récent.

### 8 ter.4 Données

- `vulnerability_scans` : début, fin, état, cause d'échec, version de Grype, date de construction de sa base, distribution et version, nombre de paquets, totaux par sévérité.
- `vulnerability_findings` : une ligne par vulnérabilité, paquet et version (unicité), avec identifiant (CVE, GHSA ou avis de distribution), sévérité (`critical`, `high`, `medium`, `low`, `negligible`, `unknown`), paquet, version installée, versions corrigées et état de correction, description, URL de référence fournies par Grype, date de publication quand la source la fournit (souvent absente du résultat de Grype), première et dernière détection, date de disparition.
- Comparaison entre deux scans réussis : nouvelles vulnérabilités, vulnérabilités disparues (paquet corrigé ou retiré), évolution des totaux. Les constats disparus et l'historique des scans suivent une rétention dédiée (proposition : 180 jours), les constats présents sont toujours conservés.

### 8 ter.5 Interface

- **Page Machine, onglet « Vulnérabilités »** : totaux (total, critiques, élevées, moyennes, faibles, inconnues), dernier scan (date, état, nombre trouvé) et prochain scan, bouton « Analyser maintenant » ; tableau CVE, sévérité, paquet, version installée, version corrigée, publication, état (`corrigeable`, `non corrigée`, `ne sera pas corrigée`, `inconnu`, d'après Grype), trié par sévérité, recherche par identifiant ou paquet, filtres par sévérité, pagination côté serveur ; détail d'une vulnérabilité (description, versions, références) avec lien vers la fiche publique quand l'URL vient d'une source reconnue (NVD, avis Debian, Ubuntu, Red Hat, Alpine, GitHub).
- **Tableau de bord** : totaux par sévérité, dernières vulnérabilités détectées (identifiant, paquet, sévérité, date) menant à l'onglet ; état distinct « aucune analyse encore » et « aucune vulnérabilité ». Pas de répartition par machine : il n'y en a qu'une (D-9).
- **Alertes** : un type de règle « vulnérabilités » (par exemple, au moins une critique corrigeable) s'ajoute au module A.

## 9. Ordre de réalisation

| Phase | Contenu | Prérequis |
|---|---|---|
| 1 | Module U : rôles, comptes multiples, TOTP, codes de secours, sessions, audit consultable, verrouillage. **Livré** (1A : rôles et comptes ; 1B : sessions, verrouillage, politique, audit ; 1C : coffre de secrets, TOTP, codes de secours, second facteur obligatoire). WebAuthn suit en phase 8 | — |
| 1 bis | Module M : redémarrer et éteindre la machine. **Livré** | Rôles (phase 1, premier incrément), polkit |
| 2 | Module J (lecture du journal). **Livré** | Groupe `systemd-journal` |
| 3 | Module S en lecture, événements `service.failed`. **Livré** | D-Bus |
| 4 | Module A : règles, cycle de vie, canaux. **Livrée** : 4A (règles, cycle de vie, acquittement, indicateurs) et 4B (canaux, silences, regroupement, renotification) | Phases 1 et 3, secrets § 2.5 |
| 5 | Module S, actions par polkit. **Livrée** (liste blanche, règle générée, « Redémarrer » depuis une alerte) | Phase 1 |
| 6 | Module B : dépôts, sauvegarde, rétention, restauration en quarantaine. **Livré** : 6A, 6B, 6C et 6D (dépôt, sources, exclusions, sauvegardes manuelles et planifiées, historique, instantanés, rétention, vérification, restauration en quarantaine, rattachement, déverrouillage, diff, dépôt de copie, SFTP) | Phases 1 et 4 |
| 7 | Module C en lecture. **Livré** | Phase 2 |
| 8 | WebAuthn, jetons d'API. **Livré** (8A : jetons ; 8B : WebAuthn comme second facteur) | Phase 1, HTTPS |
| V | Module V : vulnérabilités des paquets (Grype) de la machine du Manager, placé après la phase 4. **Livré** (dpkg, apk et RPM à base SQLite) | Phase 4 (règle d'alerte) |

Chaque phase livre son contrat dans API_CONTRACTS.md, ses tables dans DATA_MODEL.md, ses risques dans SECURITY.md, ses tests, et retire la mention « à venir » de son entrée de navigation.

## 10. Décisions ouvertes

- **[D-1] Édition des tâches Cron.** Exclue par la règle 3. Alternative possible : un catalogue de tâches prédéfinies par le projet (ex. sauvegarde, nettoyage du journal), paramétrables mais sans commande libre.
- **[D-2] Restauration en place et téléchargement d'archive.** Risque d'écrasement de données actives ; à réévaluer après la phase 6.
- **[D-3] Pré-traitements de sauvegarde.** Lesquels intégrer (export SQLite, PostgreSQL d'un conteneur) sans commande libre.
- **[D-4] Portées d'action pour les jetons d'API** (ex. redémarrer une unité depuis Home Assistant).
- **[D-5] Actions Docker** (démarrer, arrêter, redémarrer un conteneur) et **journaux de conteneur** (maquettes `d_tails_conteneur_*`, `journaux_docker_*`). La lecture des journaux est compatible avec les règles via une route en lecture seule du proxy ; les actions exigent d'élargir le proxy et une décision explicite. `exec` et la suppression restent exclus par les règles 1 et 3.
- **[D-6] Préférences système** (maquette `param_tres_syst_me_*` : nom d'hôte, fuseau, langue). Modifient la machine ; hors de ce lot.


### Décisions tranchées

- **[D-14] Connexion par passkey seule** — prise le 25 novembre 2026 en autonomie par l'agent de développement, à la demande du propriétaire d'avancer sans arbitrage, puis **confirmée par le propriétaire le 25 novembre 2026** : un administrateur l'autorise dans la politique de sécurité (désactivée par défaut). La page de connexion propose alors « Se connecter avec une passkey » : le navigateur offre les passkeys qu'il détient pour le site, sans saisie d'identifiant ; le compte est celui de la passkey choisie, qui doit vérifier l'utilisateur (code ou biométrie) et vaut donc les deux facteurs, y compris pour l'obligation de second facteur des administrateurs. Un compte désactivé ou verrouillé est refusé, un compte qui doit changer son mot de passe aussi (il se connecte avec lui). Une réponse invalide ne compte pas d'échec, son compte n'étant pas établi ; la limitation par adresse s'applique. Session de méthode `passkey`. Alternatives écartées : activé par défaut (change le niveau d'exigence sans décision d'un administrateur), réglage par compte (plus de surface pour un gain faible sur une machine domestique), saisie de l'identifiant avant la passkey (révèle les comptes existants et n'apporte rien à une passkey découvrable).
- **[D-13] Clé SSH des dépôts SFTP** — prise le 25 novembre 2026 en autonomie par l'agent de développement, à la demande du propriétaire d'avancer sans arbitrage, puis **confirmée par le propriétaire le 25 novembre 2026** : le Manager ne se sert ni de la clé ni de la configuration SSH de root. Il génère une clé Ed25519 qui lui est propre, commune à ses dépôts SFTP, remplaçable par un administrateur ; sa partie privée est scellée dans le coffre et ne va à `homelab-backup-runner` que par le fichier de secrets à usage unique d'une opération SFTP ; la clé d'hôte de chaque serveur est épinglée par l'administrateur, sans confiance à la première connexion. Alternatives écartées : la clé de root (partagée avec d'autres usages, hors du contrôle du Manager), une clé saisie par l'administrateur (secret de plus à manipuler et à transmettre), l'acceptation automatique de la clé d'hôte (exposée à l'usurpation du serveur).

- **[D-12] Planification des sauvegardes** — tranchée le 24 novembre 2026 par le propriétaire du projet : le Manager tient l'horaire réglé dans l'interface et démarre lui-même `homelab-backup@backup.service` par D-Bus ; une sauvegarde manquée pendant un arrêt du Manager est lancée une fois à son redémarrage. Pas de timer systemd généré (§ 7.1).

- **[D-11] Exécution de Grype** — tranchée le 24 novembre 2026 par le propriétaire du projet : unité systemd dédiée et durcie `homelab-vulnscan.service`, démarrée par le Manager par D-Bus avec une règle polkit limitée au verbe `start` sur cette unité (§ 8 ter.2).
- **[D-10] Mise à jour de la base de Grype** — tranchée le 24 novembre 2026 par le propriétaire du projet : automatique, une fois par jour, vers l'infrastructure d'Anchore, désactivable ; seule communication sortante autorisée en plus des notifications (§ 8 ter.2).
- **[D-9] Machines du module V** — tranchée le 24 novembre 2026 par le propriétaire du projet : Homelab Manager tourne sur une seule machine ; le module V n'analyse que celle-ci, sans agent ni hôte distant. La demande initiale (un agent Grype par machine supervisée) n'est pas retenue.

- **[D-7] Rôle des actions d'alimentation** — tranchée le 23 novembre 2026 par le propriétaire du projet : `admin` uniquement, car elles interrompent tous les services. Les `operator` n'y ont pas accès, contrairement aux actions sur les services (§ 5.2).
- **[D-8] Arrêt programmé et annulable** (`ScheduleShutdown` de logind, compte à rebours, bouton « Annuler ») — tranchée le 23 novembre 2026 : non retenu. La demande est transmise immédiatement après la confirmation par le nom de la machine.
