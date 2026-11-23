# Modèle de données — Homelab Manager

Version 2.1 — 22 novembre 2026

Modèle logique d'Ent et SQLite. Il décrit un hôte unique et ses conteneurs locaux ; il n'existe pas d'entité agent ou de relation multi-hôtes.

## Entités

### User
- UUID, identifiant de connexion unique (motif `^[a-z0-9._-]{1,64}$` pour un compte créé depuis l'interface), nom affiché, hash du mot de passe (Argon2id), timestamps.
- `role` : `viewer`, `operator` ou `admin` (contrainte `CHECK`). `disabled_at` : NULL si le compte est actif. `must_change_password` : vrai pour un compte créé par un administrateur, jusqu'à ce que son titulaire choisisse son mot de passe. `last_login_at`, `last_login_ip` : dernière connexion réussie.
- Comptes de Homelab Manager uniquement, jamais des comptes Linux. Le compte initial est créé administrateur ; la migration `003_user_accounts.sql` a promu administrateurs les comptes existants. Aucun mot de passe en clair.
- Suppression : les sessions du compte sont supprimées dans la même transaction ; les lignes d'audit, qui désignent l'acteur par son identifiant texte, sont conservées.

### Session
- UUID, référence utilisateur, hash du secret de session, expiration, révocation, timestamps.
- Ne persister que le hash du cookie de session.

### Host
- Ligne singleton de configuration/identité locale ; UUID stable, hostname, distribution, version, noyau, date de démarrage et timestamps de collecte.
- Ne pas créer de table de machines extensible à des hôtes distants.
- Les informations volatiles détaillées sont conservées dans les échantillons, pas dupliquées dans chaque ligne d'inventaire.

### Container
- UUID interne ; `docker_id` unique et stable, nom canonique, image (référence complète, tag compris, telle que renvoyée par l'inspection), état, statut, dates de création/démarrage, politique de redémarrage, ports, réseaux et montages.
- `first_seen_at`, `last_seen_at`, `removed_at` ou état équivalent pour garder l'historique de découverte sans confondre une erreur de collecte avec une suppression.
- Aucun champ de commande ou d'action arbitraire.

### SystemMetricSample
- UUID, nom de métrique, valeur numérique, unité, labels structurés limités, `recorded_at`.
- Index `(metric_name, recorded_at DESC)` et index temporel adaptés à la purge.
- Un identifiant de série ou label stable distingue cœurs CPU, systèmes de fichiers et interfaces réseau.

### ContainerMetricSample
- UUID, référence au conteneur interne, nom de métrique, valeur, unité, labels limités, `recorded_at`.
- Index `(container_id, metric_name, recorded_at DESC)` et index temporel pour la rétention.
- La suppression logique d'un conteneur conserve ses mesures jusqu'à expiration de la rétention.

### Event
- UUID, référence optionnelle au conteneur, type, sévérité, message non sensible, métadonnées bornées, `occurred_at`.
- La version actuelle enregistre les changements d'état de conteneur ; les événements hôte/collecteur ne sont pas encore persistés.

### AuditLog
- UUID, acteur local, cible (`target_type`, `target_id`), action, request ID, résultat, timestamp.
- Écrit dans la même transaction que la modification qu'il trace : modification de la rétention (`settings` / `metrics_retention_days` / `update`) et des comptes (`user` / UUID du compte / `create`, `update`, `delete`). Les autres modules d'administration v3 y journaliseront chaque action (SPEC_ADMINISTRATION.md § 2.2).

### AppSetting
- Ligne singleton (`singleton_key = 'local'`) des réglages modifiés depuis l'interface : `metrics_retention_days` (NULL = valeur de la configuration du service, contrainte `CHECK` 1–365), `updated_at`.

### Configuration locale
- Fréquences de collecte et autres paramètres résident dans la configuration du service. Seuls les réglages modifiables depuis l'interface (aujourd'hui la rétention) sont dans `app_settings`, et l'emportent sur la configuration.

## Séries temporelles et croissance

Le stockage est un fichier SQLite unique ; aucune base de séries temporelles ni serveur de base de données n'est justifié pour une seule machine. Les métriques sont horodatées en UTC et insérées par lots.

### Représentation SQLite

- UUID stockés en texte canonique, documents JSON (`labels`, `ports`, `networks`, `mounts`, `metadata`) en texte.
- Horodatages en colonnes `DATETIME` contenant du texte UTC au format `2006-01-02 15:04:05.999999999+00:00`. Le driver est configuré avec `_timezone=UTC` et `_time_format=sqlite` : l'ordre lexical est alors l'ordre chronologique, ce dont dépendent les filtres temporels, les tris et la purge. Ne jamais écrire d'horodatage par un autre chemin que l'adaptateur.
- Pragmas à l'ouverture : `foreign_keys=ON`, `journal_mode=WAL`, `synchronous=NORMAL`, `busy_timeout=5000`, transactions `IMMEDIATE`.
- Une seule connexion ouverte : les accès du Manager sont sérialisés, il n'y a pas d'erreur `SQLITE_BUSY` interne. Une transaction ne doit utiliser que son `tx`, sous peine de blocage.
- Les pages libérées par la purge sont réutilisées ; le fichier ne rétrécit pas sans `VACUUM` manuel, Manager arrêté.

- Volume : environ 870 000 échantillons hôte par jour à l'intervalle par défaut (≈ 300 Mo/jour, ≈ 2 Go pour 7 jours) ; détail et leviers dans METRICS_COLLECTION.md § 4.
- Vue courante : tous les échantillons d'un cycle partagent un même `recorded_at`, ce qui permet de lire le dernier cycle complet par égalité sur l'horodatage maximal (index `recorded_at`).
- Durée de conservation brute par défaut : 7 jours, configurable de 1 à 365 jours (configuration puis interface).
- Purger toutes les heures, par lots de 1 000 lignes sur `recorded_at`, jusqu'à épuisement ; chaque lot est une écriture courte, pour ne pas bloquer les collectes.
- Les événements et métriques sont purgés par lots selon la même rétention.
- Limiter l'intervalle et le nombre de points retournés par l'API. L'agrégation se fait en SQL à la lecture (`GROUP BY` série et intervalle, horodatage converti par `strftime('%s', substr(recorded_at, 1, 19))`) ; la pagination par curseur n'est pas implémentée.
- Mesures non disponibles : valeur absente/null ou absence d'échantillon avec fraîcheur explicite, jamais un zéro fictif.

## Intégrité et migrations

- Identifiants API UUID ; `docker_id` unique côté Docker.
- Clés étrangères (activées explicitement), contraintes de valeurs (`CHECK` sur la sévérité) et index adaptés aux filtres API.
- Horodatages UTC et migrations versionnées/reproductibles.
- Les modèles Ent restent confinés aux adaptateurs ; DTO et domaine sont indépendants.
- Les valeurs volatiles de stats ne sont pas dupliquées comme état courant dans `Container` si le dernier échantillon peut servir de source.
- `001_initial.sql` crée le schéma SQLite initial, `002_app_settings.sql` la table `app_settings`, `003_user_accounts.sql` les colonnes de rôle et d'état des comptes ; les migrations suivantes s'ajoutent sous un nouveau numéro, sans modifier les fichiers déjà livrés. Les anciennes migrations PostgreSQL ont été retirées et leurs données ne sont pas reprises.
