# Exigences de sécurité — Homelab Manager

Version 2.1 — 22 novembre 2026

## Authentification locale et HTTP

- Authentification locale par identifiant et mot de passe ; hash Argon2id recommandé.
- Premier démarrage sans `HOMELAB_ADMIN_PASSWORD` : compte par défaut `admin` / `admin`, signalé par un avertissement dans les journaux. Ce mot de passe faible doit être changé dans Paramètres immédiatement ; un nouveau mot de passe fait au moins 12 caractères. La connexion n'impose pas de longueur minimale (seulement 1 à 1 024 octets), les règles de robustesse s'appliquent à la définition du mot de passe.
- Cookies `HttpOnly`, `Secure` sous HTTPS, `SameSite` approprié, expiration et révocation de sessions.
- Protection CSRF : cookie `SameSite=Lax` et contrôle `Origin`/`Sec-Fetch-Site` sur la connexion et toutes les routes authentifiées (détail dans API_CONTRACTS.md § 3).
- Connexion : 10 tentatives par minute et par IP, réponse identique et coût Argon2id équivalent que le compte existe ou non (pas d'énumération des comptes).
- Changement de mot de passe : révocation de toutes les sessions de l'utilisateur, y compris la courante.
- Journal d'audit : toute modification faite depuis l'interface (aujourd'hui la rétention) est tracée avec l'acteur et le `requestId`.
- Modules d'administration v3 : interdits (aucun terminal, shell ni SSH ; comptes propres au Manager ; aucune exécution de code arbitraire) et modèle de privilèges (D-Bus, polkit en liste blanche, unités systemd dédiées) dans SPEC_ADMINISTRATION.md §§ 1–2.
- Erreurs publiques non bavardes : codes stables, messages génériques, `requestId` pour corréler avec les journaux.
- Validation serveur stricte, limite de taille des requêtes, timeouts, headers de sécurité et contrôle d'origine.
- Ne jamais journaliser mots de passe, cookies, tokens ou secrets.
- Le Content Security Policy n'autorise les styles inline que pour l'affichage ECharts ; les scripts inline restent interdits.

## Frontière Docker

Le socket Docker rootful permet généralement un contrôle équivalent à root sur l'hôte. L'appartenance du processus Manager au groupe `docker` n'est donc pas une permission réellement en lecture seule et ne doit pas être présentée comme telle.

| Option | Avantage | Risque |
|---|---|---|
| Accès direct au socket Unix rootful | Simple et peu de composants | Compromission du Manager = contrôle quasi root de l'hôte ; le socket ne sait pas appliquer une ACL HTTP en lecture seule |
| Proxy local restreint | Peut limiter l'API aux consultations requises | Proxy privilégié à maintenir ; une route permissive ou mal validée réintroduit le risque Docker |
| Docker rootless | Réduit l'impact au compte utilisateur dédié | Nécessite un daemon/configuration rootless et ne convient pas à tous les hôtes |

### Décision cible

Pour une installation domestique privilégiant la sécurité, utiliser un proxy local minimal, distinct du processus Manager, qui seul accède au socket Docker. Il doit appliquer une liste blanche exacte de méthodes et routes de consultation nécessaires à la liste, l'inspection et les statistiques ; refuser les autres chemins, paramètres de tunnel, méthodes mutatrices et accès aux fonctionnalités exec, archive, création, suppression, démarrage ou arrêt. Le proxy n'écoute que sur un socket Unix avec permissions restrictives, jamais sur une interface réseau.

Le proxy fourni n'autorise que `GET /v1.41/containers/json?all=1`, l'inspection d'un ID hexadécimal et `stats?stream=false`. Il filtre récursivement les champs `Env` et `Labels`, limite les réponses à 8 MiB, les appels à 10 secondes et la concurrence à 16 requêtes. Il est lancé par socket activation systemd sur `/run/homelab-docker-proxy/docker.sock` (groupe `homelab-manager`, mode `0660`) ; son unité utilise le compte et le groupe dédiés `homelab-docker-proxy`, seul membre attendu du groupe `docker`. Le Manager n'accède qu'à ce socket, par son appartenance au groupe `homelab-manager` ; le proxy n'a aucun accès aux fichiers du Manager. Vérifier les permissions effectives avant le déploiement.

Le proxy ne propose pas de flux d'événements Docker ; les changements d'état sont détectés par réconciliation périodique de l'inventaire.

Un mode direct sur socket rootful ne peut être retenu qu'en décision explicite d'exploitation, avec documentation du risque root-equivalent et sans prétendre au moindre privilège.

## Collecte et données

- Collecte strictement locale ; aucun canal de commande distant.
- Aucune exécution shell à partir des données Docker, de l'API ou de l'interface.
- Pas de fonctions de création, suppression, démarrage, arrêt ou exec de conteneurs dans le périmètre.
- Limiter et filtrer labels, noms, chemins, métadonnées et journaux ; ne pas exposer les variables d'environnement.
- Rétention bornée des métriques et événements, avec purge par lots.
- Gestion séparée des erreurs Docker/SQLite ; ne pas exposer chemin local, secrets ou détails internes.

## Système et déploiement

- Service systemd Manager sous utilisateur dédié sans groupe `docker` lorsqu'un proxy est utilisé.
- Proxy Docker isolé, socket Unix protégé et non monté/exposé au Frontend.
- Base SQLite dans `/var/lib/homelab-manager` (répertoire `0700` via `StateDirectory`, `UMask=0077`), fichier `0600` détenu par `homelab-manager` ; les fichiers `-wal`/`-shm` héritent de ce mode. Elle contient les hachages de mots de passe et de jetons de session : ne pas l'exposer, et protéger les sauvegardes comme la base.
- TLS en cas d'accès depuis le réseau local non fiable ; configuration de terminaison documentée.
- Configuration et secrets lisibles seulement par les comptes système nécessaires.
- Arrêt systemd propre : annuler et attendre les collecteurs avant de fermer la base SQLite.
