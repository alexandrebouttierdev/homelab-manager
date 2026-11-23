# Phase 3 — Services système en lecture : plan d'implémentation

> **Pour les agents :** sous-skill requis : `superpowers:executing-plans` ou `superpowers:subagent-driven-development`. Chaque tâche suit le cycle test qui échoue → implémentation → `scripts/verify.sh` → commit.

**Objectif :** livrer `SPEC_ADMINISTRATION.md` § 5.1 (module S, consultation) : inventaire des services, timers et sockets par D-Bus, compteurs, détail d'une unité avec les 50 dernières lignes de son journal, événements `service.failed` et `service.recovered`. Les actions (§ 5.2) sont la phase 5.

**Architecture :** un adaptateur `systemd` lit le gestionnaire systemd sur le bus système (`org.freedesktop.systemd1`) en lecture seule. Un `ServiceUsecase` détient une goroutine de collecte toutes les 30 s, garde le dernier inventaire en mémoire et compare l'ensemble des unités en échec à celui enregistré en base pour émettre les événements. L'API lit l'inventaire en mémoire ; seul le détail interroge D-Bus à la demande.

**Prérequis :** phase 2 (journal, pour l'extrait du détail).

## Décisions de conception

| Sujet | Choix | Motif |
|---|---|---|
| Bibliothèque | `github.com/godbus/dbus/v5` (déjà présente, v5.2.4), **pas** `go-systemd` | Seuls `ListUnits` et la lecture de propriétés sont nécessaires ; une dépendance de moins. Écart à § 2.4 de la spécification, sans effet sur les garanties |
| Accès | Bus système, sans privilège : lecture des propriétés publiques. Aucune méthode qui modifie une unité n'est appelée | § 2.1 |
| Unités | Types `service`, `timer`, `socket` chargés (`ListUnits`), 2 000 au plus | § 5.1 |
| Propriétés | `Unit` : description, états de chargement, d'activation, sous-état, état du fichier d'unité, dernier changement ; `Service` : PID principal, `MemoryCurrent`, `CPUUsageNSec`, `Result` ; `Timer` : prochaine et dernière exécution, unité déclenchée, `Result` ; `Socket` : `Result`, unités déclenchées | § 5.1 |
| Valeurs absentes | `UINT64_MAX` (comptabilité désactivée) et horodatage 0 → `null`, jamais 0 | Règle « aucune valeur inventée » |
| Collecte | Toutes les 30 s, une connexion par cycle, 25 s au plus par cycle ; état `ok`, `error` ou `unavailable` (pas de bus système) exposé avec l'inventaire | § 5.1, goroutine avec propriétaire et arrêt |
| Événements | Table `failed_units` (migration `006`) : une unité qui entre en `failed` est ajoutée et produit `service.failed` (`error`) ; quand elle redevient `active`, elle est retirée et produit `service.recovered` (`info`) ; une unité qui disparaît est retirée sans événement. Dans la même transaction que les événements. Événements sans `container_id`, `metadata` : `unit`, `activeState`, `subState`, `result` | Pas de doublon au redémarrage du Manager ni après la purge des événements |
| Détail | Lu à la demande : `FragmentPath`, `ExecStart` affiché tel quel, `Requires`, `After` (100 au plus), `Restart`, `TasksMax`, `LimitNOFILE`, `MemoryMax` (`infinity` distinct de « inconnu »), `Slice`, et 50 dernières entrées du journal de l'unité (module J) | § 5.1 |
| Nom d'unité | Motif `^[A-Za-z0-9@._:\-]{1,255}\.(service\|timer\|socket)$` et présence dans le dernier inventaire, sinon `404` | § 2.2 |
| Rôles | Tous les rôles connectés (consultation) | § 3.2 |
| Interface | Page « Services système » : compteurs, filtres par type, état et nom, tri « en échec d'abord », détail ; filtres faits par l'interface sur l'inventaire borné | § 5.1 |
| Tests | Faux `systemd` exporté par godbus sur un bus privé lancé par `dbus-daemon` (ignoré s'il manque), transitions d'état sur SQLite réel, routes `httptest` | Pas de systemd dans le conteneur de développement ni en CI |

## Routes

| Route | Rôle |
|---|---|
| `GET /api/v1/system/units` | `{ status, collectedAt, counts, units }` |
| `GET /api/v1/system/units/{unitName}` | Détail d'une unité et 50 dernières entrées de son journal |

## Tâches

1. **Domaine** : unité, valeurs absentes, compteurs, transitions d'échec (fonction pure : précédent ensemble en échec + inventaire → ajouts, retraits, événements), validation du nom.
2. **Adaptateur `systemd`** : inventaire et détail par D-Bus ; tests contre un faux gestionnaire sur un bus privé.
3. **Persistance** : migration `006_failed_units.sql`, schéma Ent, dépôt transactionnel (ensemble en échec + événements).
4. **Usecase** : collecte périodique, état, inventaire en mémoire, détail avec extrait du journal ; arrêt par contexte.
5. **HTTP et contrat** : routes, DTO, `API_CONTRACTS.md` (§ 4.7, événements § 6.2), `DATA_MODEL.md`, `SECURITY.md`.
6. **Interface** : page, navigation, événements de service dans la page Événements.
7. **État** : README, spécifications, tests ; parcours réel dans Chromium avec le binaire et un faux systemd sur un bus système privé (`DBUS_SYSTEM_BUS_ADDRESS`).

## Bilan (23 novembre 2026)

Livrée en sept commits (domaine, adaptateur, persistance, usecase, HTTP, interface, état). Constats :

- **macOS** : godbus n'y lit pas `DBUS_SYSTEM_BUS_ADDRESS` mais `DBUS_LAUNCHD_SESSION_BUS_SOCKET` (un chemin de socket). Sans effet en production (Linux) ; pour un essai local contre un faux systemd, définir les deux.
- **Chemin du socket de test** : `t.TempDir()` dépasse la limite de `sun_path` sous macOS (104 octets) ; les tests de l'adaptateur se replient alors sur un répertoire court de `/tmp`.
- **Valeurs absentes dans l'interface** : sans collecte réussie, les compteurs sont masqués plutôt qu'affichés à 0 ; un temps CPU inférieur à une seconde s'affiche en millisecondes pour ne pas ressembler à un zéro.
- **Noms échappés** : une unité dont le nom contient `\` (`systemd-fsck@dev-disk-by\x2duuid-….service`) figure dans l'inventaire, mais son détail est refusé par le motif du § 2.2 de la spécification.
- **Inventaire tronqué** : au-delà de 2 000 unités, une unité non retenue qui était enregistrée en échec est oubliée sans événement, comme une unité disparue ; les unités en échec, retenues en premier, ne sont pas concernées.
- **Parcours sur Debian 13 (OrbStack, arm64)** : binaires de production installés selon le README, unités systemd fournies, compte `homelab-manager` non root (groupes `homelab-manager` et `systemd-journal`). Vérifiés contre le vrai systemd : inventaire (112 unités), PID, mémoire et temps CPU réels, limites, extrait du journal de l'unité, `service.failed` puis `service.recovered` sur une unité de démonstration ; Docker par le proxy et journal en accès complet.
- **`ExecStart`** : le parcours réel a montré qu'un argument contenant des espaces (`sh -c "…; exit 3"`) devenait illisible une fois joint ; il est désormais placé entre guillemets (`systemctl show`, lui, ne les met pas).
- **Proxy Docker** (défaut antérieur à la phase 3) : `/run/homelab-docker-proxy` était créé `root:root 0750`, intraversable par le Manager ; l'unité du socket le crée désormais en `0755`, l'accès restant borné par le socket (`0660`, groupe `homelab-manager`).
- **Credentials systemd sous OrbStack** : la machine OrbStack est un conteneur LXC où `ImportCredential=` et `LoadCredentialEncrypted=` ne fournissent aucun `CREDENTIALS_DIRECTORY` ; la clé du coffre y a été donnée par `HOMELAB_SECRETS_KEY_FILE`. Sans effet sur une vraie Debian, non vérifiée ici.
- **systemd installé mais absent du bus** : le paquet systemd de Debian installe un fichier d'activation D-Bus pour `org.freedesktop.systemd1` qui lance `/bin/false`. Sur un bus où systemd ne tourne pas, l'appel échoue donc par `Spawn.ChildExited` et non par `ServiceUnknown` : l'inventaire passait en `error` (détail en `500`) au lieu de `unavailable` (`503`). Les erreurs `Spawn.*` valent désormais « systemd indisponible ». Le test ne passait que sur macOS, faute de ce fichier : il a cassé la CI (Ubuntu) dès la tâche 2 et a été trouvé en relançant les tests dans la machine Debian ; le bus de test n'utilise plus que son propre dossier d'activation.
- **Non vérifié** : une machine virtuelle Debian complète (non conteneurisée) et le volume d'un inventaire de serveur chargé.
