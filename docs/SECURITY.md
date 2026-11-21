# Exigences de sécurité — Homelab Manager

Version 1.1 — 20 novembre 2026

## Authentification locale
- Authentification par mot de passe administrateur local.
- Stockage avec un algorithme de hachage de mot de passe adapté (Argon2id recommandé).
- Cookies de session `HttpOnly`, `Secure` lorsque HTTPS est utilisé, et `SameSite` approprié.
- Protection CSRF pour les mutations authentifiées par cookie.
- Limitation des tentatives de connexion et délais progressifs.
- Ne jamais journaliser mot de passe, cookie, token ou secret.

## Agents
- Enrôlement contrôlé par jeton à usage limité et durée de vie courte.
- Identité propre à chaque Agent après enrôlement.
- Révocation immédiate d'un Agent.
- Connexion TLS ; authentification mutuelle ou secret Agent rotatif à préciser avant implémentation.
- Stockage des secrets avec permissions système restrictives.
- Un Agent révoqué ne peut plus publier ni recevoir de tâches.

## Autorisation des actions
- Liste blanche d'actions nommées et documentées.
- Chaque action définit ses paramètres, validation, privilèges requis, timeout et résultat.
- L'Agent vérifie lui-même que l'action est reconnue.
- Aucune commande arbitraire, aucun `sh -c` construit depuis une entrée utilisateur.
- Confirmation UI pour les actions destructrices ou à impact.
- Journalisation d'audit obligatoire.

## HTTP et WebSocket
- TLS en dehors du réseau local de confiance ; documenter la terminaison TLS.
- Limites de taille des requêtes et messages.
- Timeouts, limites de débit et nombre de connexions.
- Origines WebSocket autorisées explicitement.
- Validation stricte de toutes les entrées.
- Erreurs publiques non bavardes.
- Headers de sécurité adaptés.

## Système
- Services `systemd` exécutés avec un utilisateur système dédié et privilèges minimaux.
- PostgreSQL accessible localement au Manager uniquement.
- Fichiers de configuration et secrets non lisibles par les utilisateurs non autorisés.
- Pas de privilège root permanent pour l'Agent ; les opérations privilégiées doivent être isolées et limitées.
