# Modèle de données — Homelab Manager

Version 2.0 — 21 novembre 2026

Modèle logique d'Ent et SQLite. Il décrit un hôte unique et ses conteneurs locaux ; il n'existe pas d'entité agent ou de relation multi-hôtes.

## Entités

### User
- UUID, identifiant de connexion unique, hash du mot de passe, timestamps.
- Le MVP peut initialiser un administrateur local ; aucun mot de passe en clair.

### Session
- UUID, référence utilisateur, hash du secret de session, expiration, révocation, timestamps.
- Ne persister que le hash du cookie de session.

### Host
- Ligne singleton de configuration/identité locale ; UUID stable, hostname, distribution, version, noyau, date de démarrage et timestamps de collecte.
- Ne pas créer de table de machines extensible à des hôtes distants.
- Les informations volatiles détaillées sont conservées dans les échantillons, pas dupliquées dans chaque ligne d'inventaire.

### Container
- UUID interne ; `docker_id` unique et stable, nom canonique, image, tag si présent, état, statut, dates de création/démarrage, politique de redémarrage, ports, réseaux et montages.
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
- UUID, acteur local, cible, action, request ID, résultat, timestamp.
- Le schéma est préparé ; les opérations administratives ne sont pas encore journalisées par cette version.

### Configuration locale
- Fréquences de collecte, rétention et autres paramètres locaux résident par défaut dans la configuration du service, pas dans la base.
- N'ajouter une table de configuration que si une API d'administration persistante est spécifiée.

## Séries temporelles et croissance

Le stockage est un fichier SQLite unique ; aucune base de séries temporelles ni serveur de base de données n'est justifié pour une seule machine. Les métriques sont horodatées en UTC et insérées par lots.

### Représentation SQLite

- UUID stockés en texte canonique, documents JSON (`labels`, `ports`, `networks`, `mounts`, `metadata`) en texte.
- Horodatages en colonnes `DATETIME` contenant du texte UTC au format `2006-01-02 15:04:05.999999999+00:00`. Le driver est configuré avec `_timezone=UTC` et `_time_format=sqlite` : l'ordre lexical est alors l'ordre chronologique, ce dont dépendent les filtres temporels, les tris et la purge. Ne jamais écrire d'horodatage par un autre chemin que l'adaptateur.
- Pragmas à l'ouverture : `foreign_keys=ON`, `journal_mode=WAL`, `synchronous=NORMAL`, `busy_timeout=5000`, transactions `IMMEDIATE`.
- Une seule connexion ouverte : les accès du Manager sont sérialisés, il n'y a pas d'erreur `SQLITE_BUSY` interne. Une transaction ne doit utiliser que son `tx`, sous peine de blocage.
- Les pages libérées par la purge sont réutilisées ; le fichier ne rétrécit pas sans `VACUUM` manuel, Manager arrêté.

- Durée de conservation brute par défaut : 30 jours, configurable de 1 à 365 jours.
- Purger par lots sur `recorded_at`, avec index correspondant ; la purge ne doit pas bloquer les collectes.
- Les événements et métriques sont purgés par lots selon la même rétention.
- Limiter l'intervalle et le nombre de points retournés par l'API. Les pas agrégés et la pagination par curseur ne sont pas encore implémentés.
- Mesures non disponibles : valeur absente/null ou absence d'échantillon avec fraîcheur explicite, jamais un zéro fictif.

## Intégrité et migrations

- Identifiants API UUID ; `docker_id` unique côté Docker.
- Clés étrangères (activées explicitement), contraintes de valeurs (`CHECK` sur la sévérité) et index adaptés aux filtres API.
- Horodatages UTC et migrations versionnées/reproductibles.
- Les modèles Ent restent confinés aux adaptateurs ; DTO et domaine sont indépendants.
- Les valeurs volatiles de stats ne sont pas dupliquées comme état courant dans `Container` si le dernier échantillon peut servir de source.
- `001_initial.sql` crée le schéma SQLite complet ; les migrations suivantes s'ajoutent sous un nouveau numéro, sans modifier les fichiers déjà livrés. Les anciennes migrations PostgreSQL ont été retirées et leurs données ne sont pas reprises.
