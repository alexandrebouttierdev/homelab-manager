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
