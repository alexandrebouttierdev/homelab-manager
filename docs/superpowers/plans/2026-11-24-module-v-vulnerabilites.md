# Module V — Vulnérabilités des paquets (Grype) : plan d'implémentation

> **Pour les agents :** sous-skill requis : `superpowers:executing-plans` ou `superpowers:subagent-driven-development`. Chaque tâche suit le cycle test qui échoue → implémentation → `scripts/verify.sh` → commit en anglais (Conventional Commits) avec ses documents → push.

**Objectif :** livrer `SPEC_ADMINISTRATION.md` § 8 ter : inventaire des paquets système de la machine du Manager lu en Go, analyse par Grype dans une unité systemd dédiée et durcie démarrée par D-Bus et polkit, résultats conservés avec première et dernière détection, scans automatiques et manuels, onglet « Vulnérabilités » de la page Machine, synthèse du tableau de bord, type de règle d'alerte.

**Découpage :** deux parties, chacune utilisable seule.

- **VA — Analyse** : distributions et lecteurs de paquets, inventaire, démarrage de l'unité, lecture du résultat, stockage, scans planifiés et manuels, API, unités et règle polkit fournies, installation documentée.
- **VB — Interface et alertes** : onglet de la page Machine, section du tableau de bord, intervalle dans les Paramètres, type de règle « vulnérabilités ».

**Architecture :** un `VulnerabilityUsecase` possède une goroutine qui déclenche les scans (premier démarrage, intervalle, nouvel essai). Un scan : un port `PackageInventory` lit `/etc/os-release` et la base de paquets (lecteur dpkg, apk) et rend la distribution et les paquets ; un port `VulnerabilityScanner` écrit l'inventaire au format JSON de Syft, demande à systemd de démarrer `homelab-vulnscan.service` (D-Bus `StartUnit`), suit l'unité jusqu'à sa fin et décode le résultat de Grype, borné et strict. Une fonction pure du domaine fusionne le résultat avec les constats présents (nouveaux, toujours présents, disparus). Le Manager ne lance jamais `grype` et n'accède pas au réseau pour ce module.

**Prérequis :** phase 4 (règle d'alerte), 1B (audit). Grype figé à **v0.121.3** (publiée le 9 novembre 2026).

## Décisions de conception

| Sujet | Choix | Motif |
|---|---|---|
| Distributions (VA) | Debian, Ubuntu et dérivées (`ID` ou `ID_LIKE` contenant `debian` ou `ubuntu`) par `/var/lib/dpkg/status` ; Alpine par `/lib/apk/db/installed`. Fedora, Rocky Linux et AlmaLinux (base RPM SQLite, en-têtes binaires) : reconnues mais signalées « non prises en charge » ; lecteur RPM dans un incrément ultérieur | § 8 ter.1 : Debian 13 d'abord ; le format d'en-tête RPM demande un décodeur dédié |
| Paquets | dpkg : stanzas dont `Status` vaut `install ok installed` ; nom, version, architecture, paquet source et sa version (`Source: nom (version)`). apk : `P:`, `V:`, `A:`, `o:` (origine). 20 000 paquets au plus, fichier de 64 Mio au plus, champ de 4 Kio au plus | Grype rapproche les avis Debian par paquet source |
| Inventaire | Document JSON de Syft minimal (schéma 16.1) : `artifacts` (type `deb` ou `apk`, PURL `pkg:deb/<id>/<nom>@<version>?arch=…&upstream=…&distro=<id>-<version>`, métadonnées `dpkg-db-entry` ou `apk-db-entry`), `distro` (champs d'`os-release`), `source` de type `directory`, `descriptor` `homelab-manager`. Vérifié en le faisant relire par le décodeur de Syft v1.57.1 (celui qu'utilise Grype) et en le comparant à l'inventaire que Syft produit sur la même base dpkg ; les correspondances réelles de Grype sont vérifiées en recette, avec sa base | Format d'entrée de `grype sbom:`, sans dépendance Go à Syft |
| Échange de fichiers | Répertoire `/var/lib/homelab-vulnscan/` (`root`, 0755) : `inventory/` (`homelab-manager:homelab-vulnscan`, 2750 : le Manager écrit, l'unité lit) et `result/` (`homelab-vulnscan:homelab-manager`, 2750 : l'unité écrit, le Manager lit), créés par `tmpfiles.d`. Le Manager y écrit par `ReadWritePaths=-/var/lib/homelab-vulnscan/inventory` | Écart à la spécification (`/var/lib/homelab-manager/vulnscan/`) : le répertoire d'état du Manager est en 0700, illisible par le compte de l'unité ; aucun compte n'est ajouté au groupe de l'autre |
| Unité | `homelab-vulnscan.service`, `Type=oneshot`, compte système `homelab-vulnscan` (`sysusers.d`), commandes fixes : suppression de l'ancien résultat puis `grype sbom:/var/lib/homelab-vulnscan/inventory/inventory.json -o json --file /var/lib/homelab-vulnscan/result/result.json`. Base dans `/var/cache/homelab-vulnscan` (`GRYPE_DB_CACHE_DIR`), mise à jour automatique (`GRYPE_DB_AUTO_UPDATE=true`, désactivable par un fichier `override.conf`), base refusée au-delà de 5 jours (`GRYPE_DB_MAX_ALLOWED_BUILT_AGE=120h`), pas de recherche de nouvelle version (`GRYPE_CHECK_FOR_APP_UPDATE=false`). `TimeoutStartSec=15min`, `MemoryMax=1G`, `CPUWeight=idle`, `IOSchedulingClass=idle`, `Nice=19`, `ProtectSystem=strict`, `ProtectHome=yes`, `PrivateTmp=yes`, `NoNewPrivileges=yes`, `CapabilityBoundingSet=`, `RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX`, `ReadWritePaths=` limité au cache et à `result/` | § 8 ter.2, D-10, D-11 |
| Démarrage | Port `UnitStarter` : D-Bus `StartUnit("homelab-vulnscan.service", "replace")` sans interaction, puis lecture de `ActiveState` et `Result` toutes les 2 s jusqu'à `inactive` (succès) ou `failed`, 16 min au plus. Refus polkit → `vulnscan_not_allowed` ; unité absente → `vulnscan_not_installed`. Mécanisme réutilisé par la phase 5 | § 8 ter.2, § 2.1 |
| Règle polkit | `50-homelab-manager-vulnscan.rules` : `org.freedesktop.systemd1.manage-units` pour le seul compte `homelab-manager`, la seule unité `homelab-vulnscan.service` et le seul verbe `start` | D-11 ; fichier distinct de la règle d'alimentation, installé séparément |
| Résultat | `result.json` lu au plus 64 Mio, modifié après le début du scan (sinon `stale_result`), décodage strict des champs utiles : `matches[].vulnerability` (`id`, `severity`, `description`, `urls`, `fix.state`, `fix.versions`, `dataSource`), `matches[].artifact` (`name`, `version`, `type`), `descriptor.version`, `descriptor.db.status.built`. 50 000 constats au plus. Description bornée à 4 Kio, 10 URL de 2 Kio au plus, `http`/`https` seulement | Donnée externe validée (CODE_RULES § 8) |
| Causes d'échec | `unsupported_distro`, `inventory_failed`, `vulnscan_not_installed`, `vulnscan_not_allowed`, `timeout`, `scan_failed` (unité en échec, dont base injoignable ou périmée), `stale_result`, `invalid_result`, `interrupted` (Manager arrêté pendant un scan) | § 8 ter.3 ; le journal de l'unité donne le détail (lien vers le module J) |
| Sévérités | `critical`, `high`, `medium`, `low`, `negligible`, `unknown` (toute autre valeur → `unknown`) ; état de correction `fixed` (corrigeable), `not-fixed`, `wont-fix`, `unknown` | Valeurs de Grype |
| Constats | Unicité `(vulnerability_id, package, version)` sur les constats présents. Un constat retrouvé met à jour `last_seen_at` ; absent d'un scan réussi : `resolved_at` (disparu). Un même identifiant revenu après disparition rouvre une nouvelle ligne | § 8 ter.4 |
| Comparaison | Chaque scan réussi enregistre ses totaux par sévérité, `new_count` et `resolved_count` ; l'API rend l'évolution par rapport au scan réussi précédent | § 8 ter.4 |
| Planification | Premier scan 2 min après le démarrage du Manager s'il n'y a aucun scan réussi ; puis à l'intervalle (24 h par défaut, 6 h à 7 jours, `vulnerability_scan_interval_hours` en configuration puis modifiable dans les Paramètres, VB) compté depuis la fin du dernier scan ; un échec est retenté une fois après 15 min, puis au cycle suivant. Un scan au plus à la fois (verrou du Manager, en plus de systemd) | § 8 ter.3 |
| Scan manuel | `POST /api/v1/vulnerabilities/scans`, rôle `operator` au moins (nouvelle permission `vulnerabilities:scan`), `409 SCAN_IN_PROGRESS`, audité ; 1 scan manuel par tranche de 5 min | § 8 ter.3, § 2.2 |
| Rétention | Scans et constats disparus : 180 jours (constante), purge horaire ; constats présents jamais purgés | Proposition du § 8 ter.4 retenue |
| Indisponibilité | Sans base de données : `503 VULNERABILITIES_UNAVAILABLE`. Unité ou Grype absents : scans en échec `vulnscan_not_installed`, le reste du Manager fonctionne | § 8 ter.2 |
| Liens publics (VB) | Lien affiché seulement pour une URL de NVD, security-tracker.debian.org, ubuntu.com/security, access.redhat.com, security.alpinelinux.org ou github.com/advisories | § 8 ter.5 |
| Règle d'alerte (VB) | Type `vulnerabilities` : `severity` (minimale : `critical`, `high`, `medium`, `low`), `fixableOnly` ; vraie s'il existe au moins un constat présent correspondant dans le dernier scan réussi, observé « N vulnérabilités » ; sans scan réussi, la règle est gelée. Pas de règle par défaut | § 8 ter.5 ; migration de la contrainte `CHECK` de `alert_rules.type` |

## Données — migration `009_vulnerabilities.sql`

- `vulnerability_scans` : `id`, `trigger` (`startup`, `schedule`, `retry`, `manual`), `requested_by`, `state`, `failure`, `started_at`, `finished_at`, `grype_version`, `db_built_at`, `distro_id`, `distro_version`, `distro_name`, `package_count`, totaux par sévérité, `new_count`, `resolved_count`.
- `vulnerability_findings` : `id`, `vulnerability_id`, `severity`, `package`, `version`, `package_type`, `fix_state`, `fixed_versions` (JSON), `description`, `urls` (JSON), `data_source`, `first_seen_at`, `last_seen_at`, `resolved_at`, `first_scan_id`, `last_scan_id` ; index unique partiel `(vulnerability_id, package, version) WHERE resolved_at IS NULL`, index sur la sévérité et `resolved_at`.
- `alert_rules` reconstruite pour accepter le type `vulnerabilities` (copie, suppression, renommage, sans toucher aux alertes).
- La feuille de route décale les migrations suivantes : sauvegardes `010`, WebAuthn et jetons `011`.

## Routes

| Route | Rôle | Partie |
|---|---|---|
| `GET /api/v1/vulnerabilities/summary` | Tous | VA |
| `GET /api/v1/vulnerabilities?severity&fixable&q&state=present\|resolved&limit&offset` | Tous | VA |
| `GET /api/v1/vulnerabilities/{findingID}` | Tous | VA |
| `GET /api/v1/vulnerabilities/scans?limit` | Tous | VA |
| `POST /api/v1/vulnerabilities/scans` | `operator` | VA |
| `GET`, `PUT /api/v1/settings` (intervalle) | `admin` pour `PUT` | VB |

Erreurs nouvelles : `409 SCAN_IN_PROGRESS`, `429 SCAN_RATE_LIMITED`, `503 VULNERABILITIES_UNAVAILABLE`, `500 VULNERABILITIES_FAILED`.

## Tâches

### Partie VA

1. **Domaine** : distribution (`os-release`, famille prise en charge), paquet, sévérités, constats, fusion d'un résultat avec les constats présents, totaux et évolution, planification (prochain scan, nouvel essai), permission `vulnerabilities:scan`, causes d'échec. Tests table par table, horloge fixe.
2. **Adaptateurs** : `packages` (lecture d'`os-release`, dpkg, apk, bornes) ; `vulnscan` (écriture de l'inventaire Syft, décodage strict du résultat de Grype, démarrage et suivi de l'unité par D-Bus). Tests sur des fichiers fabriqués, un faux systemd sur un bus privé, et une comparaison avec Syft et Grype réels quand ils sont présents.
3. **Persistance** : `009_vulnerabilities.sql`, schémas Ent, dépôt (scan, application d'un résultat en une transaction, liste filtrée et paginée, synthèse, purge). Tests sur SQLite réel.
4. **Usecase** : `VulnerabilityUsecase` (goroutine, verrou, planification, scan manuel audité, interruption), synthèse et listes. Faux objets écrits à la main.
5. **HTTP et contrat** : routes, DTO, erreurs ; `API_CONTRACTS.md` (§ 4.9), `DATA_MODEL.md`, `SECURITY.md`, `ARCHITECTURE.md`, `TESTING.md`.
6. **Déploiement** : `homelab-vulnscan.service`, `tmpfiles.d`, `sysusers.d`, règle polkit, `ReadWritePaths` du Manager, configuration, README (installation de Grype : `.deb` officiel, somme de contrôle, version figée).

### Partie VB

7. **Interface** : client d'API, `src/lib/vulnerabilities.ts`, onglet « Vulnérabilités » de la page Machine (totaux, dernier et prochain scan, échec visible, « Analyser maintenant », tableau filtrable paginé, détail), section du tableau de bord.
8. **Paramètres et alertes** : intervalle des scans dans les Paramètres ; type de règle `vulnerabilities` (domaine, évaluation, formulaire).
9. **Recette** : Grype réel sur la machine de test (inventaire, base, scan, échec puis réussite), documents à jour, feuille de route, bilan dans ce plan.

## Bilan (24 novembre 2026)

VA et VB livrées en neuf commits (plan, domaine, adaptateurs, persistance, usecase et HTTP avec le déploiement, paramètres et règle d'alerte, interface, état). Écarts et constats :

- **Comptes** : le compte `homelab-vulnscan` est créé par `adduser`, comme les autres comptes du README, plutôt que par un fichier `sysusers.d`.
- **Données** : la colonne du déclencheur s'appelle `origin` (`trigger` est un mot réservé de SQLite) ; `severity_rank` trie par sévérité ; la migration `009` ajoute aussi `app_settings.vulnerability_scan_interval_hours`, ajout fait avant toute release de `009`.
- **Paquets** : comme Syft, le lecteur dpkg garde tout paquet dont les fichiers sont sur le disque (hors `deinstall`, `not-installed`, `config-files`) et le paquet source même quand il porte le nom du paquet. Sur la base dpkg du conteneur de développement (Ubuntu 24.04, 837 paquets), l'inventaire donne exactement les mêmes PURL que Syft v1.57.1, et le décodeur de Syft (celui de `grype sbom:`) le relit sans perte.
- **Démarrage de l'unité** : la fin du job de démarrage d'un service `oneshot` marque la fin du programme ; le Manager n'appelle que `StartUnit` et `GetUnit`. Le bus de test privé est partagé avec l'adaptateur systemd (`adapters/dbustest`).
- **Règle d'alerte** : paramètres `vulnSeverity` et `fixableOnly` (au lieu de `severity`, déjà propre à la règle) ; gelée tant qu'aucun scan n'a réussi.
- **Recette** (conteneur Linux de la session, Manager compilé, base neuve, chemins d'échange temporaires) : premier scan exactement 2 min après le démarrage ; inventaire réel de 837 paquets écrit en 0640 ; sans bus système, échec propre `vulnscan_not_installed` et nouvel essai planifié 15 min plus tard ; scan manuel d'un opérateur `202` puis `429` cinq minutes durant, audité ; intervalle refusé à l'opérateur (`403`), accepté pour l'administrateur et appliqué au prochain scan. Interface vérifiée dans Chromium en thème sombre et clair et sur téléphone avec un scan réussi et des constats insérés à la main dans la base de recette (aucun débordement horizontal) : échec de la dernière analyse visible à côté du dernier succès, tableau, détail et lien vers l'avis public, synthèse du tableau de bord, réglage de l'intervalle.
- **Non vérifié** : un scan Grype réel de bout en bout. Le téléchargement de la base de Grype a été refusé dans l'environnement de développement ; l'unité, `tmpfiles.d`, la règle polkit, `ReadWritePaths` et Grype 0.121.3 sont à vérifier sur la machine Debian 13 de test (installation du README, puis « Analyser maintenant »). Lecteur RPM non livré.

## Lecteur RPM (25 novembre 2026)

| Sujet | Choix | Motif |
|---|---|---|
| Base | `/var/lib/rpm/rpmdb.sqlite` (rpm 4.16 et suivants : Fedora 33+, RHEL, Rocky Linux et AlmaLinux 9+), lue par le pilote SQLite déjà utilisé (`modernc.org/sqlite`), en lecture seule (`mode=ro`, `query_only`), puis `immutable=1` si le fichier `-shm` d'une base en WAL ne peut pas être créé | Aucune dépendance nouvelle, aucune commande lancée ; lisible par tout compte |
| Berkeley DB | `/var/lib/rpm/Packages` seul (RHEL 8 et dérivées) : `unsupported_distro` | Format en fin de vie ; un décodeur Berkeley DB serait une dépendance ou un gros code de plus |
| En-têtes | Décodeur interne : nombre d'entrées et taille des données vérifiés (65 535 entrées, 64 Mio au plus), chaque décalage et chaque chaîne vérifiés avant usage ; balises 1000 (nom), 1001 (version), 1002 (release), 1003 (epoch), 1022 (architecture), 1044 (RPM source), 5096 (label de modularité) | Format documenté par rpm ; aucune lecture hors des données |
| Inventaire | Format `rpm-db-entry` de Syft ; version `[epoch:]version-release` ; PURL `pkg:rpm/<ID>/<nom>@<version>-<release>?arch=…&distro=<ID>-<VERSION_ID>&epoch=…&upstream=<RPM source>`, espace de noms `redhat` pour `rhel` | Comme Syft, pour que Grype rattache les paquets aux avis de la distribution |
| Clés | Entrées `gpg-pubkey` ignorées | Ce sont les clés de signature importées, pas des logiciels |

- Livré : lecteur, décodeur et tests sur une vraie base SQLite fabriquée par les tests (en-têtes encodés comme rpm les écrit), en-têtes malformés refusés, base Berkeley DB signalée ; inventaire Syft des paquets RPM ; message « non prise en charge » mis à jour.
- Non vérifié : une vraie base `rpmdb.sqlite` d'une distribution (aucune n'était disponible sans téléchargement) et la comparaison avec Syft sur cette base, comme pour dpkg (`HOMELAB_TEST_SYFT`, `HOMELAB_TEST_ROOT`) ; un scan Grype réel sur une machine RPM.

