# Phase 1C — Coffre de secrets et second facteur : plan d'implémentation

> **Pour les agents :** sous-skill requis : `superpowers:executing-plans` ou `superpowers:subagent-driven-development`. Chaque tâche suit le cycle test qui échoue → implémentation → `scripts/verify.sh` → commit.

**Objectif :** livrer `SPEC_ADMINISTRATION.md` § 2.5 (coffre de secrets) et § 3.3–3.4 hors WebAuthn (phase 8) : TOTP, codes de secours, connexion en deux étapes, second facteur obligatoire pour les administrateurs.

**Architecture :** l'algorithme TOTP (RFC 6238) et les codes de secours sont des fonctions pures du domaine. Un adaptateur `secretbox` chiffre en AES-256-GCM avec une clé maître importée par systemd ; un adaptateur `qrcode` produit la matrice d'un QR code. Un `SecondFactorUsecase` gère l'enrôlement en libre-service ; l'`AuthUsecase` ajoute une étape intermédiaire à la connexion. Sans clé maître, le Manager démarre en mode dégradé explicite : enrôlement et TOTP indisponibles, codes de secours toujours utilisables (ils sont hachés, pas chiffrés).

**Prérequis :** phases 1A, 1B et module M livrés.

**Écart assumé au format habituel :** comme pour la 1B, ce plan fixe la conception, les interfaces et les cas de test ; le code est écrit test d'abord pendant l'exécution.

## Décisions de conception

| Sujet | Choix | Motif |
|---|---|---|
| Clé maître | 32 octets en base64, lue dans `secrets_key_file` (`HOMELAB_SECRETS_KEY_FILE`), à défaut `$CREDENTIALS_DIRECTORY/homelab-manager-secrets-key` | § 2.5 |
| Fourniture systemd | `ImportCredential=homelab-manager-secrets-key` (systemd ≥ 254, Debian 13 : 257) plutôt que `LoadCredentialEncrypted=` | Lit `/etc/credstore.encrypted/` (chiffré par `systemd-creds`, TPM 2.0 sinon clé hôte) sans empêcher le démarrage si la clé manque ; écart de mécanisme par rapport au § 2.5, même garantie |
| Chiffrement | AES-256-GCM, nonce aléatoire de 12 octets, données associées = usage et UUID du compte (un secret ne peut pas être recopié sur un autre compte), format `v1.<base64url>` | § 2.5 |
| TOTP | Domaine, `crypto/hmac` + SHA-1, 6 chiffres, 30 s, ±1 période ; anti-rejeu par `last_used_step` mis à jour atomiquement ; secret de 20 octets | § 3.3, vecteurs RFC 6238 |
| QR code | `rsc.io/qr` v0.2.0 (2018, Go pur, aucune dépendance) : le serveur renvoie la matrice, l'interface la dessine en SVG | Pas d'image `data:` à autoriser dans la CSP ; pas de bibliothèque côté navigateur |
| Codes de secours | 10 codes `xxxx-xxxx-xxxx` (alphabet sans caractères ambigus, ≈ 59 bits), SHA-256 en base, usage unique ; régénération = remplacement | § 3.3 ; entropie suffisante pour un hachage rapide, comme les jetons de session |
| Connexion en deux étapes | Mot de passe correct → défi en mémoire (jeton 32 octets, 5 min, 5 essais, 1 000 défis au plus) dans un cookie `homelab_login` `HttpOnly` limité à `/api/v1/auth` ; `POST /auth/login/second-factor` ouvre la session | Feuille de route ; pas de table pour un état de 5 minutes ; un redémarrage impose de ressaisir le mot de passe |
| Échecs du second facteur | Comptent dans le verrouillage du compte (5 échecs → 15 min) ; le compteur n'est remis à zéro qu'à la connexion complète | Empêche de deviner un code à partir d'un mot de passe volé |
| Méthode de session | `password`, `password+totp`, `password+recovery` | `sessions.auth_method` (§ 3.6) |
| Enrôlement | Mot de passe actuel exigé pour commencer, désactiver ou régénérer les codes ; premier code exigé pour confirmer ; codes de secours affichés une seule fois à la confirmation | § 3.3 |
| Obligatoire pour les administrateurs | Booléen de la politique ; activable seulement par un administrateur qui a lui-même un second facteur (`409 SECOND_FACTOR_NOT_ENROLLED`) ; un administrateur sans facteur est ensuite limité aux routes d'enrôlement (`403 SECOND_FACTOR_REQUIRED`) | § 3.4 (« désactivé tant qu'aucun facteur n'est enrôlé ») |
| Perte de l'appareil et des codes | Hors spécification : un administrateur supprime puis recrée le compte ; pour le dernier administrateur, procédure SQLite documentée dans le README | Pas de nouvelle fonction sans décision (question Q-5) |
| Hors périmètre | WebAuthn, passkeys et jetons d'API (phase 8) ; coffre générique pour les autres modules (phases 4 et 6, réutiliseront l'adaptateur) | Feuille de route |

## Données — migration `005_second_factor.sql`

`totp_secrets` (id = UUID du compte, `ON DELETE CASCADE`, `secret_sealed`, `confirmed_at`, `last_used_step`, `created_at`) ; `recovery_codes` (UUID, `user_id` en cascade, `code_hash` unique, `used_at`, `created_at`) ; `security_policies.require_admin_second_factor` (booléen, défaut 0).

## Routes

| Route | Rôle |
|---|---|
| `POST /api/v1/auth/login` | `{ user }` sans second facteur ; sinon `{ secondFactorRequired: true, methods, expiresAt }` et cookie de défi |
| `POST /api/v1/auth/login/second-factor` | `{ method: "totp"\|"recovery", code }` → `{ user }` et cookie de session |
| `GET /api/v1/auth/second-factor` | État : activé, enrôlement en attente, codes restants, coffre disponible |
| `POST /api/v1/auth/second-factor/totp` | `{ password }` → secret, URI `otpauth://`, matrice QR |
| `POST /api/v1/auth/second-factor/totp/confirm` | `{ code }` → 10 codes de secours |
| `POST /api/v1/auth/second-factor/recovery-codes` | `{ password }` → 10 nouveaux codes |
| `POST /api/v1/auth/second-factor/disable` | `{ password }` |
| `GET\|PUT /api/v1/security/policy` | + `requireAdminSecondFactor` |

## Tâches

1. **Domaine** : TOTP (vecteurs RFC 6238 en 8 et 6 chiffres, fenêtre, URI), codes de secours (génération, normalisation, hachage), erreurs, `SecurityPolicy.RequireAdminSecondFactor`, `User.MustEnrollSecondFactor`.
2. **Adaptateurs** : `secretbox` (chiffrement, données associées, clé invalide, format) ; `qrcode` (matrice).
3. **Persistance** : migration `005`, schémas Ent, `SecondFactorRepository` (anti-rejeu atomique, usage unique des codes, cascade), politique.
4. **Usecases** : `SecondFactorUsecase` ; défi de connexion de l'`AuthUsecase` ; obligation pour les administrateurs ; contrôle de la politique.
5. **HTTP et contrat** : routes, cookie de défi, middleware d'obligation, configuration et composition, `API_CONTRACTS.md`, `SECURITY.md`, `DATA_MODEL.md`.
6. **Interface** : deuxième étape de connexion, carte « Double authentification » (QR en SVG, codes à copier), page d'enrôlement imposé, case de la politique.
7. **Déploiement et état** : unité systemd, `scripts/dev.sh`, README, spec, tests, parcours réel dans Chromium avec une application TOTP simulée.
