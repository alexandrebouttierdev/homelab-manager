# Phase 8 — Jetons d'API et WebAuthn : plan d'implémentation

> **Pour les agents :** sous-skill requis : `superpowers:executing-plans` ou `superpowers:subagent-driven-development`. Chaque tâche suit le cycle test qui échoue → implémentation → `scripts/verify.sh` → commit en anglais (Conventional Commits) avec ses documents.

**Objectif :** livrer `SPEC_ADMINISTRATION.md` § 3.3 (WebAuthn) et § 3.5 (jetons d'API).

**Découpage :**

- **8A — Jetons d'API** (aucune dépendance) : création, liste, révocation, authentification `Authorization: Bearer`, portées en lecture seule.
- **8B — WebAuthn** : passkeys et clés FIDO2 comme second facteur. Dépendance `github.com/go-webauthn/webauthn` v0.18.2 (publiée le 19 septembre 2026), qui ajoute `fxamacker/cbor`, `go-webauthn/x`, `golang-jwt/jwt/v5`, `google/go-tpm`, `tinylib/msgp`, `philhofer/fwd`, `x448/float16` et fait monter `go-viper/mapstructure` de 2.4.0 à 2.5.0 : accord du propriétaire requis pour cette montée avant l'incrément.

## Décisions de conception (8A)

| Sujet | Choix | Motif |
|---|---|---|
| Format | `hlm_` + 32 octets aléatoires en base64url (43 caractères) ; affiché une seule fois ; stocké en SHA-256 (entropie de 256 bits : pas de hachage lent nécessaire), avec un préfixe de 12 caractères pour le reconnaître | § 3.5, comme les jetons de session |
| Portées | `metrics:read` (synthèse, hôte, métriques), `containers:read`, `events:read`, `services:read` (inventaire et détail des unités, dernière action), `backups:read` (état, exécutions, instantanés ; pas la configuration) ; lecture `GET` seulement ; toute autre route répond `403 TOKEN_SCOPE_DENIED` | § 3.5, D-4 (aucune portée d'action) |
| Expiration | 1 à 365 jours, 90 par défaut ; « sans expiration » interdit | § 3.5 |
| Bornes | 20 jetons par compte, nom de 1 à 100 caractères | Ressources bornées |
| Propriété | Chacun gère ses jetons (rôle quelconque) ; un administrateur liste et révoque ceux de tous. Un jeton agit avec le compte et ses portées : un compte désactivé ou supprimé invalide ses jetons | § 3.2 « Comptes et jetons » |
| Dernier usage | Écrit au plus une fois par minute | Comme l'activité des sessions |
| Audit | Création et révocation (`api_token` / UUID) ; l'usage n'est pas audité | Volume |
| Origine | Le contrôle d'origine reste appliqué aux cookies ; un jeton ne sert qu'en `GET` | Roadmap |

## Tâches (8A)

1. Domaine : format, portées, validation, correspondance des routes.
2. Migration `013_api_tokens.sql`, schéma Ent, dépôt SQLite.
3. Usecase : création, liste, révocation, authentification.
4. HTTP : `Authorization: Bearer` dans le middleware de session, contrôle des portées, routes `/api/v1/auth/tokens`.
5. Interface : carte « Jetons d'API » des Paramètres, jeton affiché une fois.
6. Documents, `verify.sh`, recette.

## Bilan de 8A (24 novembre 2026)

- Livré comme prévu : domaine, migration `013`, dépôt, usecase, `Authorization: Bearer` dans le middleware de session avec contrôle des portées, routes `/api/v1/auth/tokens`, carte « Jetons d'API ».
- 8B attend l'accord du propriétaire sur les dépendances listées plus haut.

## Décisions de conception (8B)

Accord du propriétaire sur les dépendances le 24 novembre 2026.

| Sujet | Choix | Motif |
|---|---|---|
| Rôle | Second facteur, à côté du TOTP et des codes de secours ; pas de connexion par passkey seule | § 3.3 ; la connexion sans mot de passe reste à spécifier |
| Configuration | `webauthn_rp_id` (nom de domaine, pas d'adresse IP), `webauthn_origins` (HTTPS du domaine, ou `http://localhost`), `webauthn_rp_name` ; WebAuthn désactivé sans `webauthn_rp_id` | Exigences des navigateurs |
| Cérémonie | Attestation `none`, clé résidente préférée, vérification de l'utilisateur exigée ; session gardée en mémoire 5 minutes, une seule réponse par cérémonie | Pas de liste d'AAGUID à maintenir ; anti-rejeu |
| Connexion | Les options d'une connexion par clé s'attachent au défi du mot de passe (cookie `homelab_login`) ; la réponse passe par la route de deuxième étape existante, `method: "webauthn"` | Mêmes essais, verrouillage et limitation que le TOTP |
| Stockage | Table `webauthn_credentials` : identifiant unique, credential `go-webauthn` en JSON, nom, dates ; 10 clés par compte | Ressources bornées ; une clé ne sert qu'un compte |
| Codes de secours | Générés à l'ajout de la première clé d'un compte qui n'en a pas ; supprimés quand le dernier facteur part | Un second facteur a toujours un secours |
| Clonage | Compteur de signatures qui recule : connexion refusée | Recommandation WebAuthn |
| Audit | `webauthn_add`, `webauthn_remove` (cible `user`) ; l'usage n'est pas audité, `last_used_at` est écrit | Comme les jetons |

## Bilan de 8B (25 novembre 2026)

- Livré : adaptateur `go-webauthn` v0.18.2, migration `014`, dépôt, usecases, routes `/api/v1/auth/second-factor/webauthn…` et `/api/v1/auth/login/second-factor/webauthn`, section « Clés de sécurité et passkeys » de la carte Double authentification, connexion qui propose la clé en premier.
- Non vérifié : une vraie clé ou passkey dans un navigateur (la VM de recette est servie par adresse IP en HTTP, que WebAuthn refuse) ; à faire sur un domaine en HTTPS.
