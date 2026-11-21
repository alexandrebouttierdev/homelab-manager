# Spécification produit — Homelab Manager

Version 1.1 — 20 novembre 2026

## 1. Objectif

Homelab Manager est une application auto-hébergée de supervision et d'administration d'un homelab. Elle fournit une interface web accessible depuis les appareils du réseau local et centralise la gestion de plusieurs machines équipées d'un Agent.

Le fonctionnement courant doit rester local : pas de compte utilisateur cloud, pas de backend SaaS obligatoire et pas de dépendance à un fournisseur externe.

## 2. Utilisateurs et accès

- Un administrateur local se connecte avec un mot de passe.
- L'application ne nécessite pas de création de compte en ligne.
- L'interface doit être responsive et utilisable sur ordinateur, tablette et téléphone.
- Thèmes clair et sombre.
- Le modèle initial peut fonctionner avec un administrateur local unique ; conserver une conception permettant d'étendre les rôles plus tard sans imposer un système multi-utilisateur au MVP.

## 3. Architecture de déploiement

- Manager installé nativement sur Debian 13.
- Services lancés et supervisés par `systemd`.
- PostgreSQL installé nativement et dédié à Homelab Manager.
- Frontend servi par le Manager ou par un serveur HTTP local configuré pour le même origin ; éviter les problèmes CORS par défaut.
- Plusieurs Agents Go peuvent se connecter au Manager.
- L'Agent est installé comme service `systemd` sur les hôtes supervisés.

Aucun déploiement Docker obligatoire pour le Manager ou PostgreSQL.

## 4. Périmètre fonctionnel MVP

Le MVP vise la gestion fonctionnelle complète définie ci-dessous, pas seulement un tableau de bord en lecture seule.

### Vue d'ensemble
- État global du homelab et disponibilité des agents.
- Résumé des machines, ressources, services et conteneurs.
- Indicateurs CPU, mémoire, stockage et réseau.
- Graphiques historiques et périodes sélectionnables.
- État de fraîcheur des données et date de dernière remontée.

### Machines et agents
- Enregistrement et identification de plusieurs agents.
- État connecté / déconnecté / inconnu.
- Informations système : OS, version, architecture, noyau, uptime, CPU, mémoire, volumes et interfaces réseau.
- Inventaire et historique des métriques.
- Configuration et révocation d'un agent.
- Vue détaillée par machine.

### Services et conteneurs
- Découverte et suivi des services système autorisés.
- État des services et opérations explicitement autorisées.
- Inventaire des conteneurs Docker lorsque le connecteur correspondant est activé.
- État, ressources et opérations autorisées sur les conteneurs.
- Le support d'un runtime ne doit pas conduire à exposer son socket ou une API arbitraire à l'interface.

### Opérations et tâches
- Actions prédéfinies et validées côté Agent.
- États d'exécution : en attente, en cours, réussie, échouée, expirée, annulée si applicable.
- Résultat et historique des opérations.
- Confirmation explicite pour les actions à impact.
- Journal d'audit de l'auteur, de la cible, de l'action, de l'heure et du résultat.

### Alertes et événements
- Événements de connexion/déconnexion des agents.
- Événements de santé système et de services.
- Alertes basées sur des seuils configurables.
- Historique consultable et acquittement des alertes.

### Paramètres
- Paramètres locaux du Manager.
- Gestion des agents autorisés et de leurs capacités.
- Paramètres de collecte, rétention et seuils.
- Changement du mot de passe administrateur.

## 5. Hors périmètre / interdictions

- Aucun shell distant générique.
- Aucune exécution de commande arbitraire fournie par le navigateur.
- Aucun compte cloud obligatoire.
- Aucun accès direct du frontend à PostgreSQL ou aux agents.
- Aucun secret d'agent affiché après son enrôlement initial.
- Ne pas présumer que toutes les machines sont Debian ; l'Agent doit détecter ses capacités et annoncer les fonctionnalités supportées.

## 6. Exigences non fonctionnelles

- Sécurité par défaut : refus si action ou capacité non explicitement autorisée.
- Résilience aux déconnexions et reconnexions des agents.
- Collecte bornée en fréquence, concurrence, mémoire et taille des messages.
- Historique consultable sans charger toutes les séries en mémoire.
- Observabilité du Manager et des Agents via logs structurés.
- Migrations de schéma versionnées.
- Configuration explicite, documentée et validée au démarrage.
- Sauvegarde et restauration documentées pour PostgreSQL et la configuration.
