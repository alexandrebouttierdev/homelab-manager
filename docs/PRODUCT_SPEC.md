# Spécification produit — Homelab Manager

Version 2.1 — 22 novembre 2026

## 1. Objectif et périmètre

Homelab Manager est une application auto-hébergée de supervision d'une seule machine Linux : la machine sur laquelle le Manager est installé. Cette limite est définitive ; le produit n'est pas destiné à superviser des hôtes supplémentaires.

Le Backend Go collecte directement les données système et découvre les conteneurs Docker locaux. L'application conserve les historiques dans un fichier SQLite local et les présente dans une interface web locale. Le fonctionnement normal ne dépend d'aucun compte cloud ou service SaaS.

## 2. Déploiement cible

- Debian 13, installation native et services gérés par systemd.
- Backend Go, API REST Chi v5, Ent et SQLite embarqué (fichier unique, aucun serveur de base de données).
- Collecte système exécutée localement par le Backend.
- Découverte et statistiques Docker via l'API du démon local, au travers d'un adaptateur restreint.
- Frontend React/TypeScript/Vite avec Tailwind CSS, servi sur la même origine que l'API en production.
- Base SQLite locale au Manager ; aucun accès direct depuis le navigateur.

L'installation doit rester légère : un binaire, un fichier de base et le proxy Docker, sans Docker Compose ni service de base de données séparé.

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
- Séries historiques brutes ou agrégées (1 min, 5 min, 1 h) avec bornes temporelles contrôlées.
- Événements de changements d'état Docker et d'unités systemd en échec ou rétablies ; le statut des collecteurs est exposé par la santé et les paramètres.
- Journal systemd de la machine en lecture seule (filtres, recherche, suivi en direct, synthèse sur 24 h, export), champs sensibles masqués.
- Services, timers et sockets systemd en lecture seule (D-Bus) : inventaire, compteurs, filtres, détail d'une unité avec l'extrait de son journal, événements `service.failed` et `service.recovered`.
- Authentification locale, session, changement du mot de passe administrateur.
- Paramètres locaux de fréquence de collecte (configuration du service) et rétention réglable depuis l'interface : 7 jours par défaut, de 1 à 365 jours, effet immédiat, modification tracée dans le journal d'audit.

### Modules d'administration (v3, spécifiés, non implémentés)

Utilisateurs et accès, journaux système, services système, alertes et notifications, sauvegardes restic, tâches planifiées, redémarrage et extinction de la machine (module M, désactivé par défaut) : voir [SPEC_ADMINISTRATION.md](./SPEC_ADMINISTRATION.md), qui fixe aussi les interdits (aucun terminal, shell ni SSH ; utilisateurs propres au Manager ; aucune exécution de code arbitraire) et le modèle de privilèges. Les applications PaaS restent gérées par Coolify et sont reportées à une version ultérieure.

## 4. Sécurité et limites

- Authentification locale par identifiant et mot de passe, second facteur (application TOTP, passkey ou clé FIDO2, code de secours) si le compte en a un.
- Pas de communication de collecte entre machines.
- Pas d'exécution de commandes arbitraires sur l'hôte ou dans les conteneurs.
- Le navigateur ne reçoit jamais accès au socket Docker ou à son proxy.
- L'API du Manager expose uniquement des DTO de consultation et d'authentification.
- Les données sensibles présentes dans les métadonnées Docker, notamment les variables d'environnement, ne sont pas publiées.

L'accès Docker est une capacité privilégiée de l'hôte. La solution cible est un proxy local en liste blanche de routes de lecture, dont le risque et les limites sont décrits dans [SECURITY.md](./SECURITY.md).

## 5. Exigences non fonctionnelles

- Collecte indépendante des requêtes HTTP et de la connexion d'un utilisateur.
- Tâches d'arrière-plan bornées, annulables à l'arrêt et sans goroutine orpheline.
- Résilience aux indisponibilités temporaires de Docker, du stockage et des capteurs.
- Conservation limitée des séries brutes, purge par lots et limites de requêtes.
- Historique consultable sans chargement non borné en mémoire.
- Migrations SQLite versionnées et sauvegarde/restauration documentées.
- Empreinte mémoire faible, adaptée à un homelab.
- Interfaces responsive avec thèmes sombre et clair.

## 6. État de l'implémentation v2.1

Présent dans le dépôt : module Backend unique, authentification locale (compte initial `admin` / `admin` à changer), collecte système/Docker, API REST protégée, persistance Ent/SQLite, proxy Docker Unix filtré, interface Web et unités systemd. Le contrat exact de chaque route est API_CONTRACTS.md ; le calcul de chaque métrique est METRICS_COLLECTION.md.

Limites connues, à ne pas présenter comme fonctionnelles :

- Modules d'administration (SPEC_ADMINISTRATION.md) : sont livrés les comptes multiples et les rôles (phase 1A), les sessions, le verrouillage, la politique de sécurité et le journal d'audit consultable (phase 1B), la double authentification par application TOTP et codes de secours avec son coffre de secrets (phase 1C), la lecture du journal système (phase 2, module J), la consultation des services système (phase 3, module S en lecture) et leurs actions en liste blanche (phase 5 : démarrer, arrêter, redémarrer, recharger, exécuter un timer, « Redémarrer » depuis une alerte, règle polkit générée), les alertes et leurs notifications (phase 4 : règles, cycle de vie, acquittement, indicateurs, silences, canaux Telegram, Discord, Gotify, ntfy, SMTP et webhook signé, regroupement et renotification), les sauvegardes restic (phase 6 : dépôt local, rest-server, S3 ou SFTP avec la clé SSH propre au Manager et la clé d'hôte épinglée, sauvegardes manuelles et planifiées, historique, instantanés, rétention, vérification, restauration en quarantaine, rattachement d'un dépôt existant, diff entre instantanés, copie vers un dépôt de copie), les vulnérabilités des paquets système (module V : Grype dans l'unité dédiée `homelab-vulnscan.service`, scans planifiés et manuels, onglet de la page Machine, synthèse du tableau de bord, règle d'alerte ; Debian, Ubuntu, Alpine et distributions RPM à base SQLite) les jetons d'API en lecture seule (phase 8A), les passkeys et clés FIDO2 comme second facteur (phase 8B), les tâches planifiées en lecture seule (phase 7 : crontabs du système, scripts run-parts, timers, dernières exécutions, inspecteur d'expression) ainsi que le redémarrage et l'extinction de la machine (module M, désactivé par défaut). Aucune entrée de navigation ne reste « à venir ».
- Aucun journal de conteneur.
- Le passage de PostgreSQL à SQLite ne migre pas les données existantes.

## 7. Décisions produit

Tranchées le 22 novembre 2026 :

1. **Volume de stockage** : rétention par défaut ramenée à 7 jours (≈ 2 Go à l'intervalle de 10 s) et réglable par l'utilisateur dans l'interface.
2. **Agrégation des historiques** : livrée (API_CONTRACTS.md § 5.3).
3. **Entrées « à venir »** : spécifiées dans SPEC_ADMINISTRATION.md ; « Applications PaaS » retirée (Coolify).

Ouvertes : voir SPEC_ADMINISTRATION.md § 10.

## 8. Historique

- 2.3 — 23 novembre 2026 : sessions consultables et révocables, expiration par inactivité, verrouillage de compte, réseaux autorisés, journal d'audit consultable et exportable, audit des refus (phase 1B) ; réinitialisation de mot de passe par un administrateur et changement imposé du mot de passe par défaut ; coffre de secrets, double authentification TOTP, codes de secours et second facteur exigible des administrateurs (phase 1C) ; lecture du journal système par `journalctl` (phase 2) ; services système en lecture par D-Bus et événements d'échec des unités (phase 3).
- 2.2 — 23 novembre 2026 : comptes multiples, rôles `viewer` / `operator` / `admin`, mot de passe initial à changer, rétention réservée aux administrateurs (phase 1A des modules d'administration) ; redémarrage et extinction de la machine par logind et polkit, réservés aux administrateurs et désactivés par défaut (module M).
- 2.1 — 22 novembre 2026 : stockage SQLite embarqué, compte initial `admin` / `admin`, contrats API exacts, état `stale`, vues courantes sur le dernier cycle complet, purge horaire, agrégation des historiques et filtre par label, rétention réglable (7 jours par défaut), spécification des modules d'administration v3.
- 2.0 — 21 novembre 2026 : périmètre mono-machine, abandon de l'Agent distant.
