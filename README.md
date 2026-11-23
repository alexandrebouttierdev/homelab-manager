<div align="center">

<img src="frontend/public/favicon.svg" width="96" height="96" alt="Logo de Homelab Manager">

# Homelab Manager

**Supervision légère et auto-hébergée d'une machine Linux et de ses conteneurs Docker.**

Un binaire Go, un fichier SQLite, une interface React. Pas de cloud, pas de serveur de base de données, pas de terminal web.

[![CI](https://github.com/alexandrebouttierdev/homelab-manager/actions/workflows/ci.yml/badge.svg)](https://github.com/alexandrebouttierdev/homelab-manager/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/alexandrebouttierdev/homelab-manager?sort=semver&display_name=tag&label=release)](https://github.com/alexandrebouttierdev/homelab-manager/releases/latest)
![Go 1.26](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![React 19](https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black)
![SQLite](https://img.shields.io/badge/SQLite-embarqué-003B57?logo=sqlite&logoColor=white)
![Debian 13](https://img.shields.io/badge/Debian-13-A81D33?logo=debian&logoColor=white)

[Fonctionnalités](#fonctionnalités) · [Installation](#installation-depuis-une-release) · [Développement](#développement) · [Configuration](#configuration) · [Releases](#versions-et-releases) · [Documentation](#documentation)

</div>

---

Homelab Manager supervise **une seule machine Linux : celle qui l'héberge**. Le Backend Go collecte les métriques locales et découvre les conteneurs Docker ; un fichier SQLite conserve les mesures, les événements et le journal d'audit ; l'interface React est servie sur la même origine que l'API. Les applications PaaS et le déploiement GitOps restent confiés à Coolify.

## Fonctionnalités

<table>
<tr>
<td width="50%" valign="top">

**Supervision**

- Tableau de bord : CPU, mémoire, swap, stockage, réseau, températures réellement disponibles
- Historiques bruts ou agrégés (1 min, 5 min, 1 h), rétention réglable
- Découverte continue des conteneurs Docker, arrêtés compris, et métriques par conteneur
- Événements de création, suppression, redémarrage et changement d'état
- Journal systemd en lecture seule : filtres, recherche, suivi en direct, export, champs sensibles masqués
- Aucune valeur inventée : une donnée absente s'affiche comme indisponible

</td>
<td width="50%" valign="top">

**Administration**

- Comptes multiples, rôles `viewer`, `operator`, `admin`
- Mot de passe initial imposé, réinitialisation par un administrateur
- Double authentification : application TOTP, codes de secours, exigible des administrateurs
- Sessions consultables et révocables, expiration après inactivité
- Verrouillage après 5 échecs, réseaux autorisés (CIDR)
- Journal d'audit filtrable, export CSV et JSON
- Redémarrage et extinction de la machine (optionnel, polkit)

</td>
</tr>
</table>

**Principes de sécurité** : aucun terminal, shell ni SSH dans l'application ; comptes propres au Manager (aucun compte Linux) ; le Manager ne tourne jamais en root ; Docker n'est lu qu'au travers d'un proxy Unix en liste blanche de routes GET ; mots de passe Argon2id, cookies `HttpOnly`, contrôle d'origine. Détails dans [docs/SECURITY.md](docs/SECURITY.md).

## Architecture

```text
Navigateur ── HTTPS, même origine ──▶ homelab-manager (Go, Chi, Ent)
                                        ├── collecte /proc, /sys
                                        ├── SQLite  /var/lib/homelab-manager
                                        ├── systemd-logind via D-Bus + polkit (optionnel)
                                        └── socket Unix ──▶ homelab-docker-proxy ──▶ Docker Engine
```

| Dossier | Contenu |
|---|---|
| `backend/` | Module Go unique : API REST Chi v5, Ent/SQLite (`modernc.org/sqlite`, sans CGO), collecte Linux et Docker, architecture hexagonale |
| `frontend/` | React, TypeScript strict, Vite, React Router, TanStack Query, React Hook Form, Zod, Tailwind CSS, ECharts |
| `deployments/` | Unités systemd (Manager, proxy Docker), configuration, règle polkit |
| `scripts/` | Développement, vérification complète, construction des releases |
| `docs/` | Spécification, architecture, contrat d'API, données, sécurité, tests, règles de code |

## Installation depuis une release

Chaque release publie, pour Linux `amd64` et `arm64`, une archive contenant les deux binaires statiques, l'interface compilée, les unités systemd, la configuration et la règle polkit, ainsi qu'un fichier `SHA256SUMS`.

1. **Télécharger et vérifier** (avec la [CLI GitHub](https://cli.github.com/), qui fonctionne aussi pour un dépôt privé) :

   ```bash
   VERSION=2.3.0   # voir la dernière release
   ARCH=amd64      # ou arm64
   gh release download "v$VERSION" -R alexandrebouttierdev/homelab-manager \
     -p "homelab-manager_${VERSION}_linux_${ARCH}.tar.gz" -p SHA256SUMS
   sha256sum -c --ignore-missing SHA256SUMS
   tar -xzf "homelab-manager_${VERSION}_linux_${ARCH}.tar.gz"
   cd "homelab-manager_${VERSION}_linux_${ARCH}"
   ```

2. **Créer les comptes système** (le Manager n'est pas membre du groupe `docker`) :

   ```bash
   sudo adduser --system --group --no-create-home homelab-manager
   sudo adduser --system --group --no-create-home homelab-docker-proxy
   sudo usermod -aG docker homelab-docker-proxy   # seul compte ayant accès à Docker
   ```

3. **Installer les fichiers** :

   ```bash
   sudo install -m 0755 bin/homelab-manager bin/homelab-docker-proxy /usr/local/bin/
   sudo mkdir -p /opt/homelab-manager/frontend /etc/homelab-manager
   sudo cp -R frontend/dist /opt/homelab-manager/frontend/
   sudo install -m 0644 deployments/config/manager.yaml /etc/homelab-manager/manager.yaml
   sudo install -m 0644 deployments/systemd/* /etc/systemd/system/
   ```

   Les secrets éventuels (`HOMELAB_ADMIN_PASSWORD`, …) vont dans `/etc/homelab-manager/manager.env`, en mode `0600`.

4. **Créer la clé du coffre de secrets** (nécessaire à la double authentification) : 32 octets aléatoires, chiffrés par `systemd-creds` avec le TPM 2.0 de la machine s'il existe, sinon avec la clé de l'hôte. L'unité les importe au démarrage (`ImportCredential=`) ; la clé n'est lisible que par le service.

   ```bash
   sudo mkdir -p /etc/credstore.encrypted
   head -c 32 /dev/urandom | base64 | sudo systemd-creds encrypt \
     --name=homelab-manager-secrets-key - /etc/credstore.encrypted/homelab-manager-secrets-key
   ```

   Sans cette clé, le Manager démarre quand même : la supervision fonctionne, mais la double authentification ne peut pas être activée et les codes d'application sont refusés (les codes de secours restent valables). Le journal du service l'indique au démarrage.

5. **Démarrer** :

   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable --now homelab-docker-proxy.socket homelab-manager.service
   journalctl -u homelab-manager -n 20   # la version démarrée y est journalisée
   ```

6. Ouvrir `http://<machine>:8080`. Sans `HOMELAB_ADMIN_PASSWORD`, le premier compte est `admin` / `admin` et **l'interface impose d'en choisir un autre dès la première connexion**.

**Mise à jour** : sauvegarder la base (voir plus bas), arrêter `homelab-manager.service`, remplacer les binaires et `/opt/homelab-manager/frontend/dist`, puis redémarrer. Les migrations SQLite s'appliquent seules au démarrage.

## Développement

Prérequis : Go `1.26.10` ou plus récent, Node.js 22 et npm. Docker Engine est optionnel : sans lui, la collecte Docker est simplement indisponible. Les tests d'intégration du journal système lancent le vrai `journalctl` sur des fichiers fabriqués par `systemd-journal-remote` (`sudo apt install systemd-journal-remote`) ; sans lui, ils sont ignorés.

1. Copier [.env.example](.env.example) vers `.env`. Laisser `HOMELAB_ADMIN_PASSWORD` vide pour se connecter avec `admin` / `admin` (changement imposé à la première connexion), ou y définir un secret fort.
2. Installer les paquets de l'interface :

   ```bash
   npm --prefix frontend ci
   ```

3. Lancer le Backend et Vite :

   ```bash
   scripts/dev.sh
   ```

   Vite écoute sur `http://127.0.0.1:5173` et transmet `/api` au Manager sur le port 8080. La base SQLite est créée au premier démarrage (`backend/data/homelab-manager.db` en développement) et les migrations versionnées sont appliquées automatiquement. Le script crée aussi une clé de coffre de développement, `backend/data/secrets.key` (non versionnée), pour essayer la double authentification.

Pour lancer les processus séparément :

```bash
set -a; source .env; set +a
go -C backend run ./cmd/manager
npm --prefix frontend run dev -- --host 127.0.0.1
```

Si la base ne peut pas être ouverte (répertoire non accessible en écriture, disque plein, migration en échec), le Manager répond sur `/health` et rapporte son état dégradé sur `/api/v1/health`, mais l'authentification et la supervision persistée ne sont pas disponibles. Corriger la cause puis redémarrer le Manager.

### Vérification

```bash
scripts/verify.sh
```

Le script enchaîne `gofmt`, `go vet`, `go test` avec et sans `-race`, Prettier, ESLint, TypeScript, Vitest, le build de l'interface et le test des scripts de release. Il doit passer avant chaque commit. Les règles de contribution sont dans [AGENTS.md](AGENTS.md) et [docs/CODE_RULES.md](docs/CODE_RULES.md) : code et commits en anglais, documentation et interface en français, [Conventional Commits](https://www.conventionalcommits.org/fr/).

### Construire depuis les sources

```bash
npm --prefix frontend ci
npm --prefix frontend run build
go -C backend build -o bin/homelab-manager ./cmd/manager
go -C backend build -o bin/homelab-docker-proxy ./cmd/docker-proxy
```

Ou, pour produire les mêmes archives qu'une release : `scripts/release/build.sh 2.3.0 dist`.

## Configuration

La configuration est lue dans `manager.yaml` ([deployments/config/manager.yaml](deployments/config/manager.yaml)), cherché dans `/etc/homelab-manager`, puis `./deployments/config`, puis le répertoire courant. Les variables `HOMELAB_*` l'emportent sur le YAML ; [.env.example](.env.example) les documente pour le développement.

| Variable | Rôle |
|---|---|
| `HOMELAB_HTTP_ADDRESS` | Adresse d'écoute |
| `HOMELAB_DATABASE_PATH` | Fichier SQLite (défaut `./data/homelab-manager.db`, `/var/lib/homelab-manager/homelab-manager.db` en production) ; répertoire créé en `0750`, fichier en `0600` |
| `HOMELAB_ADMIN_USERNAME` / `HOMELAB_ADMIN_PASSWORD` | Création de l'administrateur au premier démarrage, s'il n'existe pas encore (le mot de passe d'un compte existant n'est jamais modifié). Sans mot de passe, `admin` / `admin` avec changement imposé à la première connexion ; au démarrage, un compte qui accepte encore `admin` reçoit la même obligation. Un mot de passe configuré fait 12 à 1 024 octets, haché avec Argon2id |
| `HOMELAB_TLS_CERT_FILE` / `HOMELAB_TLS_KEY_FILE` | Active HTTPS et les cookies `Secure` |
| `HOMELAB_SESSION_SECURE` | Force l'attribut `Secure` si TLS est terminé par un proxy |
| `HOMELAB_SYSTEM_COLLECT_INTERVAL_SECONDS` | Collecte système (5–300 s, défaut 10) |
| `HOMELAB_DOCKER_COLLECT_INTERVAL_SECONDS` | Statistiques Docker (5–300 s, défaut 15) |
| `HOMELAB_DOCKER_DISCOVERY_INTERVAL_SECONDS` | Inventaire Docker (10–600 s, défaut 30) |
| `HOMELAB_METRICS_RETENTION_DAYS` | Rétention par défaut des métriques (1–365 jours, défaut 7) ; une valeur choisie dans Paramètres l'emporte et persiste |
| `HOMELAB_DOCKER_PROXY_SOCKET` | Socket du proxy local restreint |
| `HOMELAB_DOCKER_ENGINE_SOCKET` | Socket Engine lu par le proxy, pas par le Manager |
| `HOMELAB_WEB_ROOT` | Répertoire `dist` servi par le Manager |
| `HOMELAB_SECRETS_KEY_FILE` | Fichier de la clé du coffre de secrets (32 octets en base64). Vide : la clé importée par systemd, `$CREDENTIALS_DIRECTORY/homelab-manager-secrets-key`. Absente ou invalide : double authentification indisponible, le reste fonctionne |
| `HOMELAB_POWER_ACTIONS_ENABLED` | Autorise Redémarrer / Éteindre depuis la page Machine (défaut `false`) ; exige aussi la règle polkit |

Ne pas enregistrer les secrets de production dans Git. Protéger le fichier d'environnement du service.

## Exploitation

### Proxy Docker et permissions

Le socket du proxy (`/run/homelab-docker-proxy/docker.sock`) appartient au groupe `homelab-manager` (mode `0660`) : le Manager y accède par son groupe principal, le proxy n'a aucun droit sur les fichiers du Manager. Le proxy écoute seulement sur Unix, autorise les routes GET nécessaires, filtre `Env` et `Labels`, borne les réponses à 8 MiB et la concurrence à 16 requêtes. L'unité du Manager crée `/var/lib/homelab-manager` (`StateDirectory`, mode `0700`). Vérifier les permissions effectives et les journaux systemd après installation.

### Réseaux autorisés et récupération

Dans « Utilisateurs & accès » › Politique, un administrateur peut limiter l'accès à des réseaux (CIDR). Le Manager voit l'adresse de la connexion : derrière un proxy inverse, il faut autoriser le proxy, et la liste ne filtre alors plus les clients. Une liste qui exclurait l'administrateur courant est refusée, mais un changement d'adresse ultérieur peut tout de même bloquer l'accès. Pour vider la liste (la politique est gardée en mémoire : le service doit être arrêté) :

```bash
sudo systemctl stop homelab-manager
sudo -u homelab-manager sqlite3 /var/lib/homelab-manager/homelab-manager.db \
  "UPDATE security_policies SET allowed_networks = '[]';"
sudo systemctl start homelab-manager
```

Un compte verrouillé après 5 mots de passe faux se déverrouille seul au bout de 15 minutes, ou immédiatement si un administrateur réinitialise son mot de passe.

### Journaux système

La page « Journaux système » lit le journal systemd par `journalctl`, en lecture seule, pour tous les rôles. L'unité fournie ajoute le groupe `systemd-journal` au service (`SupplementaryGroups=`) : sans lui, le Manager ne voit que ses propres entrées et la page l'indique. `journalctl` est cherché dans le `PATH` du service ; absent, la page affiche le journal comme indisponible et le reste fonctionne.

Le Manager ne lance que `journalctl`, sans shell, avec des options construites à partir de filtres validés et sans lui transmettre ses variables d'environnement. Chaque appel est limité à 10 secondes. `_CMDLINE` et les champs dont le nom contient `PASSWORD`, `TOKEN` ou `SECRET` sont masqués ; un secret écrit dans le texte d'un message reste visible, comme dans `journalctl`. L'export est limité aux 50 000 entrées les plus récentes du filtre.

### Double authentification

Chaque utilisateur l'active dans « Configuration » › Double authentification : mot de passe actuel, QR code à scanner (Aegis, Google Authenticator, 1Password…), premier code, puis 10 codes de secours affichés une seule fois. La connexion demande ensuite un code après le mot de passe. Dans « Utilisateurs & accès » › Politique, un administrateur qui l'a lui-même activée peut l'exiger de tous les administrateurs : ceux qui ne l'ont pas encore ne peuvent plus que l'activer.

**Appareil perdu** : se connecter avec un code de secours, puis désactiver et réactiver la double authentification (ou régénérer les codes). **Appareil et codes perdus** : un autre administrateur supprime le compte et le recrée. S'il s'agit du dernier administrateur, retirer son second facteur en base, service arrêté :

```bash
sudo systemctl stop homelab-manager
sudo -u homelab-manager sqlite3 /var/lib/homelab-manager/homelab-manager.db <<'SQL'
DELETE FROM recovery_codes WHERE user_id = (SELECT id FROM users WHERE username = 'admin');
DELETE FROM totp_secrets WHERE id = (SELECT id FROM users WHERE username = 'admin');
SQL
sudo systemctl start homelab-manager
```

Cette intervention n'est pas tracée dans le journal d'audit. **Clé du coffre perdue ou remplacée** : les secrets TOTP existants deviennent illisibles ; chaque utilisateur se connecte avec un code de secours, puis désactive et réactive la double authentification. Sauvegarder la clé chiffrée avec la base ne suffit pas toujours : liée au TPM, elle ne se déchiffre que sur cette machine.

### Redémarrer et éteindre depuis l'interface (optionnel)

Désactivé par défaut. Pour l'activer :

```bash
sudo apt install polkitd
sudo install -o root -g root -m 0644 deployments/polkit/50-homelab-manager-power.rules /etc/polkit-1/rules.d/
```

puis mettre `power_actions_enabled: true` dans `/etc/homelab-manager/manager.yaml` et redémarrer `homelab-manager.service`. La règle n'autorise que le redémarrage et l'extinction, par le seul compte `homelab-manager`, et respecte les verrous d'inhibition. Seuls les administrateurs voient la carte « Alimentation » de la page Machine ; la confirmation demande de saisir le nom de la machine. Après une extinction, il faut un accès physique (ou Wake-on-LAN configuré hors de Homelab Manager) pour rallumer la machine.

### Sauvegarde et restauration

La base tient dans un fichier, accompagné de `-wal` et `-shm` pendant l'exécution. Ne pas copier ces fichiers à chaud : utiliser la sauvegarde en ligne de SQLite (paquet `sqlite3`), qui produit un fichier cohérent sans arrêter le Manager :

```bash
sudo -u homelab-manager sqlite3 /var/lib/homelab-manager/homelab-manager.db \
  ".backup '/var/lib/homelab-manager/backup-$(date +%F).db'"
```

Pour restaurer : arrêter `homelab-manager.service`, remplacer `homelab-manager.db` par la sauvegarde, supprimer les éventuels `homelab-manager.db-wal`/`-shm`, rétablir le propriétaire `homelab-manager` et le mode `0600`, puis redémarrer. Sauvegarder avant toute mise à niveau.

## Versions et releases

Chaque push sur `main` publie une release GitHub ([.github/workflows/release.yml](.github/workflows/release.yml)) après une vérification complète. La version suit [SemVer](https://semver.org/lang/fr/) et se déduit des messages Conventional Commits depuis le dernier tag `v*` :

| Commits depuis la dernière release | Version |
|---|---|
| `type!:` ou `BREAKING CHANGE:` | majeure (`3.0.0`) |
| au moins un `feat:` | mineure (`2.4.0`) |
| autres (`fix:`, `docs:`, `refactor:`…) | correctif (`2.3.1`) |

La première release est `v2.3.0`, alignée sur la [spécification produit](docs/PRODUCT_SPEC.md). Les notes sont générées à partir des commits, groupées par type. La version est injectée à la construction : elle s'affiche dans l'interface et dans les journaux du service ; une construction locale affiche `dev`. Les scripts sont dans [scripts/release/](scripts/release/).

## État et limites connues

- **Livré** : supervision système et Docker, historique, événements, comptes et rôles, sessions, politique de sécurité, journal d'audit, double authentification (TOTP et codes de secours), journaux système, redémarrage et extinction (optionnels).
- **Spécifié, pas encore implémenté** : passkeys (WebAuthn) et jetons d'API, services systemd, alertes et notifications, sauvegardes restic, tâches planifiées. Voir [docs/SPEC_ADMINISTRATION.md](docs/SPEC_ADMINISTRATION.md) et la [feuille de route](docs/superpowers/plans/2026-11-23-feuille-de-route-v3.md).
- Historiques bruts ou agrégés, 2 000 points au plus par requête ; l'interface choisit le pas selon la période.
- À l'intervalle par défaut (10 s), la base croît d'environ 300 Mo par jour jusqu'à la rétention : ≈ 2 Go pour 7 jours.
- Listes bornées, sans pagination ; images, volumes et réseaux Docker déduits des conteneurs.
- SQLite n'accepte qu'un écrivain à la fois : le Manager sérialise ses accès sur une connexion unique, ce qui convient à une seule machine.
- Les rafraîchissements automatiques d'un onglet ouvert comptent comme une activité de session ; le journal d'audit n'est pas purgé.
- Le passage de PostgreSQL à SQLite n'a pas migré les données : une ancienne installation PostgreSQL repart d'une base vide.

## Documentation

| Document | Contenu |
|---|---|
| [Spécification produit](docs/PRODUCT_SPEC.md) | Périmètre, décisions, état |
| [Modules d'administration](docs/SPEC_ADMINISTRATION.md) | Comptes, sessions, alimentation, modules v3 à venir |
| [Architecture](docs/ARCHITECTURE.md) | Couches, collecte, planification |
| [Contrats API](docs/API_CONTRACTS.md) | Routes, DTO, codes d'erreur exacts |
| [Modèle de données](docs/DATA_MODEL.md) | Tables SQLite, migrations |
| [Collecte et formules](docs/METRICS_COLLECTION.md) | Calcul de chaque métrique |
| [Sécurité](docs/SECURITY.md) | Exigences et frontières |
| [Stratégie de tests](docs/TESTING.md) | Couverture et vérifications manuelles |
| [Règles de code](docs/CODE_RULES.md) | Go, TypeScript, Git |
