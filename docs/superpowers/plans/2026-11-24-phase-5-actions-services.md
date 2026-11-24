# Phase 5 — Actions sur les services système : plan d'implémentation

> **Pour les agents :** sous-skill requis : `superpowers:executing-plans` ou `superpowers:subagent-driven-development`. Chaque tâche suit le cycle test qui échoue → implémentation → `scripts/verify.sh` → commit en anglais (Conventional Commits) avec ses documents.

**Objectif :** livrer `SPEC_ADMINISTRATION.md` § 5.2 et l'action « Redémarrer » du § 6.4 : `start`, `stop`, `restart`, `reload` sur les unités d'une liste blanche, demandés à systemd par D-Bus et autorisés par une règle polkit générée à partir de la configuration ; « Exécuter » un timer ; confirmation, audit, rôle `operator` au moins, résultat suivi jusqu'à l'état stable.

**Architecture :** un `UnitActionUsecase` vérifie la demande (verbe, nom, liste blanche, débit, action en cours), écrit l'audit, puis suit l'action dans une goroutine qu'il possède, bornée à 90 s et arrêtée avec le Manager. Un port `UnitController` demande le verbe à systemd (`StartUnit`, `StopUnit`, `RestartUnit`, `ReloadUnit`, mode `replace`, sans interaction) et lit le job puis l'état de l'unité jusqu'à un état stable. L'état de la dernière action par unité est gardé en mémoire et servi à l'interface, qui l'interroge jusqu'à la fin.

**Prérequis :** phase 1 (rôles, audit), phase 3 (inventaire), mécanisme D-Bus et polkit du module V.

## Décisions de conception

| Sujet | Choix | Motif |
|---|---|---|
| Liste blanche | Réglage de service `systemd_manageable_units` (YAML, ou `HOMELAB_SYSTEMD_MANAGEABLE_UNITS` séparé par des espaces), vide par défaut. Chaque nom suit le motif du § 2.2 ; un modèle sans instance (`nom@.service`) est refusé ; doublons retirés ; 200 noms au plus. Un nom invalide empêche le démarrage | § 5.2 ; la configuration du service est sous le contrôle de root, pas de l'interface |
| Règle polkit | Générée par la sous-commande `homelab-manager polkit-rule`, qui lit la même configuration et écrit sur la sortie standard la règle `50-homelab-manager.rules` : `org.freedesktop.systemd1.manage-units` pour le seul compte `homelab-manager`, les seules unités de la liste et les seuls verbes `start`, `stop`, `restart`, `reload`. L'administrateur l'installe dans `/etc/polkit-1/rules.d/` et la régénère après chaque changement de liste | § 2.1 (« générée à partir de la configuration ») ; option A retenue le 24 novembre 2026 par le propriétaire du projet : la règle ne peut pas diverger de la liste, que le Manager ne peut pas relire dans `/etc/polkit-1/rules.d/` |
| Double contrôle | Le Manager vérifie lui-même la liste blanche avant tout appel D-Bus ; polkit la vérifie à nouveau. Une liste élargie sans règle régénérée échoue `not_allowed` | Défense en profondeur (§ 2.1) |
| Verbes | `start`, `stop`, `restart`, `reload`, sur un service, un timer ou un socket de la liste. Aucun autre appel mutateur | § 5.2 |
| Timer « Exécuter » | Côté interface : `start` du service que le timer déclenche (`triggers`), proposé seulement si ce service est dans la liste blanche | § 5.2 ; aucune route ni règle supplémentaire |
| Suivi | Après l'appel, lecture de l'état du job toutes les secondes jusqu'à sa disparition, puis de `ActiveState` jusqu'à un état stable (ni `activating`, ni `deactivating`, ni `reloading`), 90 s au plus | § 5.2 |
| Résultat | `succeeded` si l'unité n'est pas `failed` à l'état stable ; sinon `failed` avec la cause : `failed` (unité en échec), `not_allowed` (refus polkit), `not_found` (unité inconnue), `rejected` (systemd refuse le job : verbe non applicable, unité masquée, démarrage manuel interdit…), `timeout`, `unavailable` (bus ou systemd injoignable), `interrupted` (Manager arrêté) | Message explicite dans l'interface |
| Concurrence | Une action en cours par unité (`409 ACTION_IN_PROGRESS`) ; 30 demandes par minute et par utilisateur (`429 RATE_LIMITED`) | § 2.2 |
| Audit | Ligne `system_unit`, nom de l'unité, verbe, `requested` écrite avant l'appel (sans écriture, rien n'est demandé), puis `success` ou `failure` à la fin | § 2.2 ; même modèle que le module M |
| Rôle | Nouvelle permission `services:manage`, accordée à `operator` et `admin` | § 5.2 |
| Mise à jour de l'inventaire | L'inventaire reste collecté toutes les 30 s ; l'interface affiche l'état lu à la fin de l'action et relit l'inventaire | Pas de collecte supplémentaire |
| Données | Aucune table : l'état des actions est en mémoire (une entrée par unité de la liste), l'historique est dans le journal d'audit | Pas de nouvelle migration |
| Alerte | Action « Redémarrer » sur une alerte `unit_failed` dont l'unité est dans la liste blanche, pour un `operator` ; même confirmation et même suivi | § 6.4 |
| Module V | `vulnscan.UnitStarter` reste inchangé : ses erreurs sont propres aux scans. Le nouveau contrôleur reprend sa façon d'appeler systemd | Pas de régression du module V |

## Routes

| Route | Rôle |
|---|---|
| `GET /api/v1/system/units` (ajout de `manageableUnits`) | Tous |
| `POST /api/v1/system/units/{unitName}/actions` `{ "action": "restart" }` → `202` | `operator` |
| `GET /api/v1/system/units/{unitName}/actions/last` | Tous |

## Tâches

1. **Domaine** : verbes, états et causes d'une action, liste blanche validée, permission `services:manage`. Tests de validation.
2. **Adaptateur** `systemd.Controller` : appels D-Bus, suivi du job et de l'état, classement des erreurs. Tests sur un bus privé (`dbustest`) avec un faux systemd.
3. **Usecase** `UnitActionUsecase` : contrôles, audit, goroutine possédée, débit, état en mémoire. Tests avec un faux contrôleur.
4. **HTTP** : routes, DTO, erreurs, `manageableUnits`. Tests `httptest`.
5. **Configuration et sous-commande** `polkit-rule` : lecture de la liste, génération de la règle. Tests de la règle produite.
6. **Interface** : boutons d'action de la page Services (détail d'une unité, « Exécuter » d'un timer), confirmation, suivi, message de résultat ; « Redémarrer » depuis une alerte. Logique dans les hooks et `src/lib/`. Tests Vitest.
7. **Documents** : API_CONTRACTS § 4.7, SPEC_ADMINISTRATION (§ 5.2, § 6.4, § 9), SECURITY, DATA_MODEL (aucune table), README (liste blanche, génération et installation de la règle), PRODUCT_SPEC, `deployments/config/manager.yaml`.

## Bilan (24 novembre 2026)

- Livré comme prévu : domaine (`unit_action.go`), contrôleur D-Bus `systemd.Controller`, `UnitActionUsecase`, routes, sous-commande `polkit-rule`, boutons de la page Services et « Redémarrer » des alertes. Aucune nouvelle dépendance ni migration.
- Écart : le code d'erreur de limitation de débit est `UNIT_ACTION_RATE_LIMITED` et non `RATE_LIMITED`, dont le message de l'interface parle des tentatives de connexion.
- Recette dans la VM Debian 13 (OrbStack, systemd et polkitd 126 réels) sur l'unité `hm-demo.service` : sans règle, `not_allowed` ; unité hors liste, `409 UNIT_NOT_MANAGEABLE` ; règle générée par `homelab-manager polkit-rule` installée, `restart`, `stop` et `start` aboutis avec l'état final lu ; `reload` d'un service sans `ExecReload` : `rejected` ; audit `requested` puis `success` ou `failure`.
- Reste à faire hors de cette phase : faire reposer `vulnscan.UnitStarter` sur le contrôleur commun ; vérifier « Exécuter » d'un timer réel en recette.
