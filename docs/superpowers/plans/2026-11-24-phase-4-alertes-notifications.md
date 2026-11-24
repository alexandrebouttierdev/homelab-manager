# Phase 4 — Alertes et notifications : plan d'implémentation

> **Pour les agents :** sous-skill requis : `superpowers:executing-plans` ou `superpowers:subagent-driven-development`. Chaque tâche suit le cycle test qui échoue → implémentation → `scripts/verify.sh` → commit en anglais (Conventional Commits) avec ses documents → push. Les tests réels se font dans la machine Debian OrbStack `homelab-manager`, pas sous macOS.

**Objectif :** livrer `SPEC_ADMINISTRATION.md` § 6 (module A) : règles évaluées toutes les 15 s sur les données déjà collectées, cycle `firing` → `acknowledged` → `resolved` avec historique et indicateurs, silences, canaux de notification (Telegram, Discord, Gotify, ntfy, SMTP, webhook signé), regroupement et renotification.

**Découpage :** la phase est livrée en deux parties, chacune utilisable seule.

- **4A — Moteur et cycle de vie** : règles, alertes, acquittement, indicateurs, interface. Aucune communication sortante.
- **4B — Notifications** : canaux, envois, regroupement, renotification, silences, bouton « Tester ». C'est la seule communication réseau sortante du Manager.

**Architecture :** un `AlertUsecase` possède une goroutine d'évaluation toutes les 15 s (10 s au plus par cycle). Il lit, par des ports en lecture, le dernier cycle de métriques hôte et conteneurs, les conteneurs, l'état des collecteurs et l'inventaire systemd en mémoire (phase 3). Une fonction pure du domaine compare les conditions vraies aux alertes ouvertes et produit les transitions. Les alertes ouvertes sont en base, ce qui évite les doublons au redémarrage ; le début d'une condition qui n'a pas encore tenu sa durée est gardé en mémoire. En 4B, un `NotificationUsecase` reçoit les transitions par une file bornée, les regroupe par canal et par minute, puis les envoie par des adaptateurs sortants (`net/http`, `net/smtp`).

**Prérequis :** 1C (coffre de secrets), 3 (inventaire systemd), 1B (audit).

## Décisions de conception

| Sujet | Choix | Motif |
|---|---|---|
| Dépendances | Aucune nouvelle : `net/http`, `net/smtp`, `crypto/tls`, `crypto/hmac`, `path` (motifs) de la bibliothèque standard | Règle « dépendance justifiée » ; les six canaux sont de simples requêtes HTTP ou SMTP |
| Types de règle | `host_metric`, `container_metric`, `container_state`, `unit_failed`, `collector`. Le type « Sauvegarde » arrive avec la phase 6, qui en a les données | § 6.1 ; pas de règle sans donnée |
| Métriques | Celles des catalogues § 5.4 et § 5.5 de `API_CONTRACTS.md` (liste blanche dans le domaine), plus une métrique dérivée par le moteur : `memory.available_percent` = `memory.available / memory.total × 100` (hôte) | Règle par défaut « mémoire disponible < 10 % » sans changer la collecte |
| Cibles | Une alerte par règle et par cible : une série de métrique (labels), un conteneur, une unité, un collecteur. Clé de cible stable (`host:filesystem.percent{mountpoint=/}`, `container:<uuid>`, `unit:<nom>`, `collector:docker`) | Une règle « tous les conteneurs » produit une alerte par conteneur |
| Évaluation | Toutes les 15 s, sur le dernier cycle de moins de 5 min (`LatestMetrics`, et une nouvelle lecture groupée du dernier cycle de chaque conteneur) ; aucune requête d'historique | § 6.1 « sans requête historique lourde » |
| Durée | La condition doit rester vraie à chaque évaluation pendant `forSeconds` (0 à 86 400) ; le début est gardé en mémoire et perdu au redémarrage, la durée recommence alors | Simple, sans table ; une alerte déjà ouverte reste ouverte |
| Valeur absente | Métrique absente, collecteur périmé ou cible inconnue : ni déclenchement ni résolution. Cible disparue (conteneur supprimé, système de fichiers démonté, unité déchargée), règle désactivée ou supprimée : résolution avec motif | Règle « aucune valeur inventée » |
| Cycle de vie | `firing` → `acknowledged` (auteur, date) → `resolved` (date, motif `condition_cleared`, `target_gone`, `rule_disabled`, `rule_deleted`). Une condition à nouveau vraie après résolution ouvre une nouvelle alerte | § 6.2 |
| Unicité | Index unique partiel `(rule_id, target_key)` sur les alertes non résolues | Pas de doublon, même après un redémarrage |
| Sévérités | `info`, `warning`, `critical` (alertes) ; distinctes des sévérités d'événements | § 6.1 |
| Règles par défaut | Insérées par la migration `007`, modifiables et désactivables, pas supprimables (`409 DEFAULT_RULE`) : disque `filesystem.percent > 85` 5 min `warning` ; `memory.available_percent < 10` 5 min `warning` ; `temperature.celsius > 85` 2 min `critical` (voir Q-6) ; collecteur système ou Docker indisponible 2 min `warning` ; unité en échec, motif `*`, immédiat, `critical` | § 6.1 |
| Indicateurs | Alertes ouvertes par sévérité, résolues sur 24 h, durée moyenne de résolution sur 24 h | § 6.2 |
| Rétention | Les alertes résolues suivent la rétention des métriques et des événements (purge horaire) ; les alertes ouvertes ne sont jamais purgées | Base bornée |
| Rôles | Consultation : tous les rôles. Acquitter, poser et lever un silence : `alerts:operate` (`operator`, `admin`). Règles et canaux : `alerts:manage` (`admin`) | § 3.2 |
| Audit | Création, modification et suppression de règle ou de canal, acquittement, silence posé ou levé, envoi de test | § 2.2, phase 1B |
| Silences (4B) | Portée : une règle, une cible ou tout ; 15 min à 7 jours ; auteur et motif (1 à 200 caractères) obligatoires ; levée anticipée possible. Une alerte sous silence reste ouverte et visible (`silenced: true`), mais ne notifie pas | § 6.2 ; le silence agit sur les notifications, pas sur la supervision |
| Canaux (4B) | Telegram (jeton du bot, identifiant du chat), Discord (URL du webhook), Gotify (URL du serveur, jeton d'application), ntfy (URL du serveur, sujet, jeton facultatif), SMTP (hôte, port, `starttls` ou `tls`, utilisateur, mot de passe, expéditeur, 1 à 10 destinataires), webhook (URL, secret HMAC facultatif). 20 canaux au plus | § 6.3 |
| Secrets (4B) | Jetons, URL de webhook Discord, mot de passe SMTP et secret HMAC scellés par le coffre (données associées : `notification-channel:<uuid>`). Jamais renvoyés : l'API renvoie `defined` et les 12 premiers caractères hexadécimaux du SHA-256. Sans clé maître : `503 SECRETS_UNAVAILABLE` à la création et aux envois, la supervision continue | § 2.5 |
| Routage (4B) | Chaque canal a une sévérité minimale et une renotification (`renotifyMinutes`, 0 = jamais, 5 à 1 440, défaut 30) pour une alerte `firing` non acquittée | § 6.3 |
| Regroupement (4B) | File bornée de 200 transitions par canal ; envoi au plus 1 fois par minute et par canal, en un seul message ; au-delà, les plus anciennes sont remplacées par « N autres alertes » | § 6.3 |
| Envois (4B) | Délai de 10 s par envoi, TLS vérifié (1.2 au moins), aucune redirection suivie, réponse lue sur 64 Kio au plus. Un échec est noté sur le canal (`lastErrorAt`, erreur sans URL ni jeton) sans nouvel essai : la renotification couvre les alertes qui restent ouvertes | § 6.3 « n'affectent pas la supervision » |
| Webhook (4B) | `POST` JSON versionné (`version: 1`, hôte, alertes avec règle, sévérité, état, cible, valeur, seuil, dates). Avec un secret : en-têtes `X-Homelab-Timestamp` et `X-Homelab-Signature: sha256=<hex>` sur `timestamp + "." + corps` | § 6.3, signature HMAC-SHA256 |
| Adresses (4B) | Schémas `http` et `https` seulement, sans identifiants dans l'URL ; les adresses privées sont permises (Gotify et ntfy sont souvent sur le réseau local), la configuration est réservée aux administrateurs | Usage homelab |
| Actions depuis une alerte | Liens vers le détail de l'unité, son journal (module J) et le conteneur. « Redémarrer » arrive avec la phase 5 | § 6.4 |
| Interface | Page « Alertes » : indicateurs, alertes ouvertes puis historique, acquittement ; onglets « Règles », « Silences » (4B), « Canaux » (4B). Formulaires React Hook Form + Zod ; logique dans `useAlertsPage.ts` et `src/lib/alerts.ts`. Retire « Alertes » de « à venir » | Règles du projet |

## Questions à trancher

| Id | Question | Proposition retenue si rien n'est décidé |
|---|---|---|
| Q-6 | La règle par défaut « température > seuil du capteur » : le collecteur n'enregistre pas les seuils `high` / `critical` des capteurs | Seuil fixe de 85 °C, modifiable. Collecter les seuils des capteurs serait une évolution de la collecte, à décider à part |
| Q-7 | Un silence masque-t-il aussi l'alerte dans l'interface ? | Non : elle reste listée avec la mention « sous silence », seules les notifications sont suspendues |
| Q-8 | Telegram et Discord ne peuvent être vérifiés qu'avec de vrais jetons | Tests automatiques contre des serveurs `httptest` ; vérification réelle de ntfy et du webhook dans la machine de test ; Telegram, Discord, Gotify et SMTP réels restent à vérifier par le propriétaire |

## Routes

| Route | Rôle | Partie |
|---|---|---|
| `GET /api/v1/alerts?state=open\|resolved\|all&from&to&limit` | Tous | 4A |
| `GET /api/v1/alerts/summary` | Tous | 4A |
| `GET /api/v1/alerts/{alertID}` | Tous | 4A |
| `POST /api/v1/alerts/{alertID}/acknowledge` | `alerts:operate` | 4A |
| `GET /api/v1/alert-rules`, `GET /api/v1/alert-rules/{ruleID}` | Tous | 4A |
| `POST /api/v1/alert-rules`, `PUT /api/v1/alert-rules/{ruleID}`, `DELETE /api/v1/alert-rules/{ruleID}` | `alerts:manage` | 4A |
| `GET /api/v1/alert-silences` | Tous | 4B |
| `POST /api/v1/alert-silences`, `DELETE /api/v1/alert-silences/{silenceID}` | `alerts:operate` | 4B |
| `GET /api/v1/notification-channels` | `alerts:manage` | 4B |
| `POST`, `PUT /{channelID}`, `DELETE /{channelID}` sur `/api/v1/notification-channels` | `alerts:manage` | 4B |
| `POST /api/v1/notification-channels/{channelID}/test` | `alerts:manage` | 4B |

Erreurs nouvelles : `409 ALERT_ALREADY_RESOLVED`, `409 DEFAULT_RULE`, `409 TOO_MANY_RULES` (200 au plus), `409 TOO_MANY_CHANNELS`, `503 SECRETS_UNAVAILABLE`, `502 NOTIFICATION_FAILED` (test seulement, message sans secret).

## Données

- `007_alerts.sql` (4A) : `alert_rules` (`id`, `name`, `type`, `severity`, `enabled`, `is_default`, `params` JSON, `created_at`, `updated_at`), `alerts` (`id`, `rule_id`, `rule_name`, `rule_type`, `severity`, `target_key`, `target` JSON, `state`, `value`, `threshold`, `observed`, `fired_at`, `acknowledged_at`, `acknowledged_by`, `resolved_at`, `resolution`), index unique partiel `(rule_id, target_key) WHERE state <> 'resolved'`, index sur `fired_at` et `resolved_at`. Règles par défaut insérées avec des UUID fixes.
- `008_notifications.sql` (4B) : `alert_silences` (`id`, `scope` `rule`/`target`/`all`, `rule_id`, `target_key`, `reason`, `created_by`, `starts_at`, `ends_at`, `lifted_at`), colonne `last_notified_at` ajoutée à `alerts`, `notification_channels` (`id`, `name`, `type`, `enabled`, `min_severity`, `renotify_minutes`, `settings` JSON non secret, `secrets` scellé, `last_sent_at`, `last_error_at`, `last_error`, `created_at`, `updated_at`).
- La feuille de route décale les migrations suivantes : sauvegardes `009`, WebAuthn et jetons `010`.

## Tâches

### Partie 4A

1. **Domaine des alertes** : types de règle et validation des paramètres (liste blanche des métriques, opérateurs, labels, durées, motifs d'unité avec `path.Match`), clés de cible, métrique dérivée `memory.available_percent`, fonction pure `EvaluateAlerts(rules, snapshot, open, pending, now)` → déclenchements, résolutions avec motif et nouvel état « en attente », indicateurs ; permissions `alerts:operate` et `alerts:manage`. Tests table par table, horloge fixe.
2. **Persistance** : `007_alerts.sql`, schémas Ent, dépôt SQLite (règles, alertes ouvertes, application des transitions dans une transaction, acquittement, historique filtré, indicateurs, purge) ; lecture groupée du dernier cycle de chaque conteneur dans le dépôt de supervision. Tests sur SQLite réel : unicité après redémarrage, règle par défaut non supprimable.
3. **Usecase** : `AlertUsecase` avec goroutine toutes les 15 s (propriétaire `main.go`, arrêt par contexte, 10 s par cycle), lecture par ports des métriques, conteneurs, état des collecteurs et inventaire systemd ; CRUD des règles avec audit ; acquittement ; purge horaire. Faux objets écrits à la main.
4. **HTTP et contrat** : routes 4A, DTO, autorisations, erreurs ; `API_CONTRACTS.md` (nouveau § 4.8), `DATA_MODEL.md`, `SECURITY.md`, `ARCHITECTURE.md`.
5. **Interface 4A** : client d'API, `src/lib/alerts.ts` (libellés, tri, durées, description d'une règle et d'une cible, schémas Zod des règles), `useAlertsPage.ts`, page « Alertes » (indicateurs, ouvertes, historique, acquittement selon le rôle), onglet « Règles » (liste, formulaire par type, désactivation), navigation.
6. **Recette 4A dans la VM** : binaire déployé, règles par défaut actives, unité de démonstration en échec → alerte critique puis résolution, disque factice au-dessus du seuil (règle sur un point de montage de test), conteneur arrêté, acquittement, aucun doublon après un redémarrage du Manager ; `README`, `PRODUCT_SPEC`, `SPEC_ADMINISTRATION` (§ 6 « livré en partie »), `TESTING`, feuille de route.

### Partie 4B

7. **Domaine des notifications** : silences (portées, bornes, chevauchement, alerte couverte), canaux (types, paramètres, sévérité minimale, renotification), file regroupée par canal (bornée, « N autres »), messages en français (titre, sévérité, cible, valeur et seuil, durée), corps JSON du webhook et signature HMAC. Tests purs.
8. **Persistance 4B** : `008_notifications.sql`, schémas Ent, dépôts des silences et des canaux (secrets scellés par le coffre, empreinte), état du dernier envoi.
9. **Adaptateurs sortants** : un `Notifier` par type derrière un port unique, client HTTP borné (délai, TLS, pas de redirection, réponse limitée), SMTP `starttls` / `tls` ; erreurs nettoyées de toute URL et de tout jeton. Tests avec `httptest` et un faux serveur SMTP en mémoire.
10. **Usecase 4B** : `NotificationUsecase` (goroutine de regroupement à la minute, file bornée, renotification, silences appliqués, échec sans effet sur l'évaluation), CRUD des canaux et des silences avec audit, envoi de test ; branché sur les transitions de l'`AlertUsecase`.
11. **HTTP et contrat 4B** : routes, DTO sans secret, `API_CONTRACTS.md` (§ 4.8, webhook documenté), `SECURITY.md` (seule communication sortante, secrets, adresses), `DATA_MODEL.md`.
12. **Interface 4B** : onglets « Silences » (poser depuis une alerte, durée, motif, levée) et « Canaux » (formulaire par type, secrets « défini » avec empreinte, bouton « Tester », dernier envoi et dernière erreur).
13. **Recette 4B dans la VM** : ntfy en conteneur et récepteur de webhook local (vérification de la signature), regroupement d'une rafale en un message, renotification, silence, clé maître absente ; documents à jour, feuille de route (phase 4 livrée), bilan dans ce plan.

## Bilan 4A (24 novembre 2026)

Livrée en six commits (domaine, persistance, usecase, HTTP, interface, état). Écarts et constats :

- **Données** : `alerts` garde la cible en JSON (`target` : genre, métrique, labels, conteneur, unité, collecteur) pour les liens de l'interface, plutôt qu'un simple libellé ; `observed` porte l'état qui a déclenché une règle sans valeur numérique. `last_notified_at` ne sert qu'aux notifications : il arrive avec `008` en 4B.
- **Résolutions** : en plus de celles prévues, `no_data` (série absente d'un cycle à jour depuis 5 min, par exemple un disque démonté) et `rule_changed` (type, sévérité ou paramètres modifiés ; un renommage garde les alertes). Une cible sortie de l'inventaire (conteneur supprimé, unité déchargée) est résolue aussitôt (`target_gone`).
- **Sources gelées** : une source illisible ou périmée ne déclenche ni ne résout ; vérifié en arrêtant le proxy Docker : l'alerte d'un conteneur arrêté est restée ouverte pendant la panne, la collecte Docker est passée en alerte au bout de 2 min puis s'est résolue au rétablissement.
- **Interface** : le formulaire édite un seul label par règle (l'API en accepte 5) ; les métriques sont proposées triées par leur nom français et restent valides quand le type change.
- **Recette dans la machine OrbStack** (binaire et interface déployés, 29 vérifications) : 6 règles par défaut, règle par défaut non supprimable (`409`), unité `hm-demo` tuée → alerte critique `failed (signal)`, aucun doublon après un redémarrage du Manager, acquittement puis résolution au redémarrage de l'unité, `409` sur une alerte résolue ; règle sur `memory.available_percent` (métrique dérivée) déclenchée, renommée sans résolution, seuil changé → `rule_changed` sans nouveau déclenchement ; règle d'état de conteneur sur `crashy` ; supprimée → `rule_deleted` ; indicateurs, audit, Manager non root, aucune connexion sortante. Interface vérifiée dans le navigateur (alerte réelle avec lien vers l'unité, onglet Règles, affichage mobile).
- **Non vérifié** : les règles par défaut de disque et de température sur des données réelles (la machine OrbStack, conteneur LXC, n'expose ni partition physique ni capteur) ; elles sont couvertes par les tests du moteur. Thème clair non revu.
