# Spécification produit — Homelab Manager

Version 2.0 — 21 novembre 2026

## 1. Objectif et périmètre

Homelab Manager est une application auto-hébergée de supervision d'une seule machine Linux : la machine sur laquelle le Manager est installé. Cette limite est définitive ; le produit n'est pas destiné à superviser des hôtes supplémentaires.

Le Backend Go collecte directement les données système et découvre les conteneurs Docker locaux. L'application conserve les historiques dans PostgreSQL et les présente dans une interface web locale. Le fonctionnement normal ne dépend d'aucun compte cloud ou service SaaS.

## 2. Déploiement cible

- Debian 13, installation native et services gérés par systemd.
- Backend Go, API REST Chi v5, Ent et PostgreSQL dédié.
- Collecte système exécutée localement par le Backend.
- Découverte et statistiques Docker via l'API du démon local, au travers d'un adaptateur restreint.
- Frontend React/TypeScript/Vite avec Tailwind CSS, servi sur la même origine que l'API en production.
- PostgreSQL local au Manager ; aucun accès direct depuis le navigateur.

Docker Compose peut servir au développement ou aux tests, mais ne constitue pas un mode de déploiement de production requis.

## 3. Fonctionnalités

### Tableau de bord
- Résumé de l'état et de la fraîcheur des données de l'hôte.
- Ressources CPU, mémoire, swap, stockage, réseau et températures réellement disponibles.
- Nombre de conteneurs Docker par état et indicateurs agrégés utiles.
- Signaler explicitement les données indisponibles ou périmées ; aucune valeur fictive.

### Machine
- Identité, distribution Linux, version, noyau, démarrage et uptime.
- Historique des ressources système et interfaces réseau.
- Détails des systèmes de fichiers et capteurs effectivement accessibles.
- État du collecteur et dernière collecte réussie.

### Docker
- Découverte automatique et continue de tous les conteneurs locaux, y compris les conteneurs arrêtés.
- Liste, détail, état, configuration d'image, ports, réseaux et volumes observables.
- Métriques courantes et historiques indépendantes pour chaque conteneur.
- Historique des créations, suppressions, redémarrages et changements d'état.
- Affichage des journaux de conteneurs hors périmètre initial.
- Aucune fonction de création, suppression, démarrage, arrêt, exec ou administration Docker.

### Historique, événements et paramètres
- Séries historiques brutes avec bornes temporelles contrôlées.
- Événements de changements d'état Docker ; le statut des collecteurs est exposé par la santé et les paramètres.
- Authentification locale, session, changement du mot de passe administrateur.
- Paramètres locaux de fréquence de collecte et de rétention, avec bornes et valeurs par défaut documentées.

Les alertes à seuil configurables ne sont pas incluses dans cette version de spécification, sauf si elles sont ultérieurement définies par une exigence produit explicite.

## 4. Sécurité et limites

- Authentification locale par identifiant et mot de passe.
- Pas de communication de collecte entre machines.
- Pas d'exécution de commandes arbitraires sur l'hôte ou dans les conteneurs.
- Le navigateur ne reçoit jamais accès au socket Docker ou à son proxy.
- L'API du Manager expose uniquement des DTO de consultation et d'authentification.
- Les données sensibles présentes dans les métadonnées Docker, notamment les variables d'environnement, ne sont pas publiées.

L'accès Docker est une capacité privilégiée de l'hôte. La solution cible est un proxy local en liste blanche de routes de lecture, dont le risque et les limites sont décrits dans [SECURITY.md](./SECURITY.md).

## 5. Exigences non fonctionnelles

- Collecte indépendante des requêtes HTTP et de la connexion d'un utilisateur.
- Tâches d'arrière-plan bornées, annulables à l'arrêt et sans goroutine orpheline.
- Résilience aux indisponibilités temporaires de Docker, PostgreSQL et des capteurs.
- Conservation limitée des séries brutes, purge par lots et limites de requêtes.
- Historique consultable sans chargement non borné en mémoire.
- Migrations PostgreSQL versionnées et sauvegarde/restauration documentées.
- Interfaces responsive avec thèmes sombre et clair.

## 6. État de l'implémentation v2.0

Les fondations décrites sont présentes dans le dépôt : module Backend unique, authentification locale, collecte système/Docker, API REST protégée, persistance Ent/PostgreSQL, proxy Docker Unix filtré, interface Web et unités systemd. La migration `002_single_host.sql` retire les tables métier multi-Agents ; sauvegarder la base avant mise à niveau.

Les limites actuellement connues sont l'absence de pagination par curseur et d'agrégation des historiques, la liste des conteneurs/événements plafonnée sans pagination, et l'absence de tests d'intégration PostgreSQL exécutés sans base dédiée. Les fonctionnalités non présentes ne doivent pas être présentées comme opérationnelles.
