# Modèle de données — Homelab Manager

Version 2.0 — 21 novembre 2026

Modèle logique d'Ent et PostgreSQL. Il décrit un hôte unique et ses conteneurs locaux ; il n'existe pas d'entité agent ou de relation multi-hôtes.

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
- Fréquences de collecte, rétention et autres paramètres locaux résident par défaut dans la configuration du service, pas nécessairement dans PostgreSQL.
- N'ajouter une table de configuration que si une API d'administration persistante est spécifiée.

## Séries temporelles et croissance

La première version utilise PostgreSQL standard ; aucune base de séries temporelles supplémentaire n'est justifiée. Les métriques sont horodatées en UTC et insérées par lots.

- Durée de conservation brute par défaut : 30 jours, configurable de 1 à 365 jours.
- Purger par lots sur `recorded_at`, avec index correspondant ; la purge ne doit pas bloquer les collectes.
- Les événements et métriques sont purgés par lots selon la même rétention.
- Limiter l'intervalle et le nombre de points retournés par l'API. Les pas agrégés et la pagination par curseur ne sont pas encore implémentés.
- Mesures non disponibles : valeur absente/null ou absence d'échantillon avec fraîcheur explicite, jamais un zéro fictif.

## Intégrité et migrations

- Identifiants API UUID ; `docker_id` unique côté Docker.
- Clés étrangères, contraintes de valeurs et index adaptés aux filtres API.
- Horodatages UTC et migrations versionnées/reproductibles.
- Les modèles Ent restent confinés aux adaptateurs ; DTO et domaine sont indépendants.
- Les valeurs volatiles de stats ne sont pas dupliquées comme état courant dans `Container` si le dernier échantillon peut servir de source.
- La migration `002_single_host.sql` supprime les anciennes tables métier multi-Agents et leurs historiques ; sauvegarder la base avant mise à niveau.
