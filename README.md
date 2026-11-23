# Homelab Manager

Homelab Manager supervise une seule machine Linux : celle qui héberge le Manager. Le Backend Go collecte les métriques locales et découvre les conteneurs Docker ; un fichier SQLite local conserve les mesures et événements ; l'interface React est servie sur la même origine que l'API.

La spécification produit v2.1 est dans [docs/PRODUCT_SPEC.md](docs/PRODUCT_SPEC.md). Les règles de contribution sont dans [CLAUDE.md](CLAUDE.md).

## Architecture

- `backend/` : module Go unique, API REST Chi v5, Ent/SQLite, collecte Linux et Docker.
- `frontend/` : React, TypeScript strict, Vite, React Router, TanStack Query, Zod, React Hook Form, Tailwind CSS et ECharts.
- `deployments/systemd/` : unités Manager et proxy Docker local en liste blanche.
- `docs/` : architecture, contrats API, données, sécurité, métriques et stratégie de tests.

Aucun Agent distant, enrôlement, protocole Manager-Agent, commande distante ou fonction d'administration des conteneurs n'est inclus. Aucun serveur de base de données ni Docker Compose n'est requis.

## État fonctionnel et limites connues

Le code v2.1 est en place : authentification locale, collecteurs système/Docker, API protégée, historique SQLite, événements de changement de conteneur, interface dashboard/hôte/containers/images/volumes/réseaux Docker/stockage/métriques/réseau/événements/paramètres, proxy Docker Unix filtré et unités systemd. L'interface utilise Tailwind CSS et des composants React internes ; les écrans sans support Backend restent explicitement marqués « à venir ».

Les limites actuelles sont explicites :

- Les historiques sont bruts ou agrégés (1 min, 5 min, 1 h), 2 000 points au plus par requête ; l'interface choisit le pas selon la période.
- À l'intervalle par défaut (10 s), la base croît d'environ 300 Mo par jour jusqu'à la rétention : ≈ 2 Go pour les 7 jours par défaut. La rétention se règle dans Paramètres.
- Les modules d'administration (utilisateurs, journaux, services, alertes, sauvegardes, Cron, redémarrage et extinction de la machine) sont spécifiés dans [docs/SPEC_ADMINISTRATION.md](docs/SPEC_ADMINISTRATION.md) mais pas implémentés. Les applications restent gérées par Coolify.
- Les listes sont bornées et ne fournissent pas encore de pagination.
- SQLite n'accepte qu'un écrivain à la fois ; le Manager sérialise ses accès sur une connexion unique, ce qui convient au volume d'une seule machine.
- L'installation système (comptes, permissions et groupes) doit être réalisée par l'administrateur avant d'activer les unités systemd.

Le stockage est passé de PostgreSQL à SQLite. **Les données d'une installation PostgreSQL existante ne sont pas migrées** : le Manager repart d'une base vide, l'administrateur est recréé (identifiants par défaut `admin` / `admin`, ou `HOMELAB_ADMIN_PASSWORD` s'il est défini). L'ancien service PostgreSQL peut être arrêté puis désinstallé une fois la nouvelle version validée.

## Prérequis

- Go `1.26.10` ou plus récent.
- Node.js 22 et npm.
- Docker Engine optionnel ; la collecte Docker passe par `homelab-docker-proxy.socket`.

## Développement

1. Copier [.env.example](.env.example) vers `.env`. Laisser `HOMELAB_ADMIN_PASSWORD` vide pour se connecter avec `admin` / `admin`, ou y définir un secret fort.
2. Installer les paquets Frontend :

   ```bash
   npm --prefix frontend ci
   ```

3. Lancer le Backend et Vite :

   ```bash
   scripts/dev.sh
   ```

   Vite écoute sur `http://127.0.0.1:5173` et transmet `/api` au Manager sur le port 8080. La base SQLite est créée au premier démarrage (`backend/data/homelab-manager.db` en développement) et les migrations versionnées sont appliquées automatiquement.

Pour lancer les processus séparément :

```bash
set -a
source .env
set +a
go -C backend run ./cmd/manager
npm --prefix frontend run dev -- --host 127.0.0.1
```

Si la base ne peut pas être ouverte (répertoire non accessible en écriture, disque plein, migration en échec), le Manager répond sur `/health` et rapporte son état dégradé sur `/api/v1/health`, mais l'authentification et la supervision persistée ne sont pas disponibles. Corriger la cause puis redémarrer le Manager.

## Construction et vérification

```bash
npm --prefix frontend run lint
npm --prefix frontend run typecheck
npm --prefix frontend run test
npm --prefix frontend run build

go -C backend test ./...
go -C backend test -race ./...
go -C backend vet ./...
go -C backend build ./cmd/manager ./cmd/docker-proxy
```

Le script [scripts/verify.sh](scripts/verify.sh) lance les vérifications disponibles.

## Configuration

La configuration de développement est documentée dans [.env.example](.env.example) et [deployments/config/manager.yaml](deployments/config/manager.yaml). Les variables `HOMELAB_*` remplacent les paramètres YAML correspondants.

- `HOMELAB_HTTP_ADDRESS` : adresse d'écoute.
- `HOMELAB_DATABASE_PATH` : chemin du fichier SQLite (défaut `./data/homelab-manager.db`, `/var/lib/homelab-manager/homelab-manager.db` en production). Le répertoire est créé en `0750` et le fichier en `0600`.
- `HOMELAB_ADMIN_USERNAME` / `HOMELAB_ADMIN_PASSWORD` : création de l'administrateur au premier démarrage, uniquement s'il n'existe pas encore (un compte existant n'est jamais modifié). Sans mot de passe configuré, les identifiants par défaut sont **`admin` / `admin`** et un avertissement est journalisé : **changer ce mot de passe dans Paramètres dès la première connexion**, surtout si l'interface est accessible depuis le réseau. Un mot de passe configuré ou choisi dans Paramètres fait 12 à 1 024 octets ; tous sont hachés avec Argon2id.
- `HOMELAB_TLS_CERT_FILE` / `HOMELAB_TLS_KEY_FILE` : active HTTPS et les cookies `Secure`.
- `HOMELAB_SESSION_SECURE` : force l'attribut `Secure` si TLS est terminé par un proxy.
- `HOMELAB_SYSTEM_COLLECT_INTERVAL_SECONDS` : collecte système (5–300 s, défaut 10).
- `HOMELAB_DOCKER_COLLECT_INTERVAL_SECONDS` : statistiques Docker (5–300 s, défaut 15).
- `HOMELAB_DOCKER_DISCOVERY_INTERVAL_SECONDS` : inventaire Docker (10–600 s, défaut 30).
- `HOMELAB_METRICS_RETENTION_DAYS` : rétention par défaut des métriques (1–365 jours, défaut 7). Une valeur choisie dans Paramètres l'emporte et persiste.
- `HOMELAB_DOCKER_PROXY_SOCKET` : socket du proxy local restreint.
- `HOMELAB_DOCKER_ENGINE_SOCKET` : socket Engine lu par le proxy, pas par le Manager.
- `HOMELAB_WEB_ROOT` : répertoire `dist` servi par le Manager.

Ne pas enregistrer les secrets de production dans Git. Protéger le fichier d'environnement du service.

## Déploiement natif Debian

Le Manager n'a pas besoin d'être membre du groupe `docker`. Pour Docker, compiler les deux binaires et installer les assets Frontend :

```bash
npm --prefix frontend ci
npm --prefix frontend run build
go -C backend build -o bin/homelab-manager ./cmd/manager
go -C backend build -o bin/homelab-docker-proxy ./cmd/docker-proxy
```

Installer les binaires sous `/usr/local/bin`, le contenu de `frontend/dist` sous `/opt/homelab-manager/frontend/dist`, et la configuration sous `/etc/homelab-manager/` : `manager.yaml` (copie de [deployments/config/manager.yaml](deployments/config/manager.yaml)) et, pour les secrets, `manager.env` en mode `0600`. Les variables d'environnement l'emportent sur le YAML. Le Manager cherche `manager.yaml` dans `/etc/homelab-manager`, puis `./deployments/config`, puis le répertoire courant.

Comptes système :

```bash
sudo adduser --system --group --no-create-home homelab-manager
sudo adduser --system --group --no-create-home homelab-docker-proxy
sudo usermod -aG docker homelab-docker-proxy   # seul compte ayant accès à Docker
```

Le socket du proxy appartient au groupe `homelab-manager` (mode `0660`) : le Manager y accède par son groupe principal, le proxy n'a aucun droit sur les fichiers du Manager.

Installer et activer les unités `homelab-docker-proxy.socket` et `homelab-manager.service`. Le socket proxy écoute seulement sur Unix, autorise les routes GET nécessaires, filtre `Env` et `Labels`, borne la taille des réponses à 8 MiB et la concurrence à 16 requêtes. Vérifier les permissions effectives avant le démarrage.

L'unité systemd crée `/var/lib/homelab-manager` (`StateDirectory`, mode `0700`). Le Manager y crée la base et applique les migrations au démarrage. Contrôler les journaux systemd après installation.

### Sauvegarde et restauration

La base tient dans un fichier, accompagné de `-wal` et `-shm` pendant l'exécution. Ne pas copier ces fichiers à chaud : utiliser la sauvegarde en ligne de SQLite (paquet `sqlite3`), qui produit un fichier cohérent sans arrêter le Manager :

```bash
sudo -u homelab-manager sqlite3 /var/lib/homelab-manager/homelab-manager.db \
  ".backup '/var/lib/homelab-manager/backup-$(date +%F).db'"
```

Pour restaurer : arrêter `homelab-manager.service`, remplacer `homelab-manager.db` par la sauvegarde, supprimer les éventuels `homelab-manager.db-wal`/`-shm`, rétablir le propriétaire `homelab-manager` et le mode `0600`, puis redémarrer. Sauvegarder avant toute mise à niveau.

## Documentation

- [Spécification produit](docs/PRODUCT_SPEC.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Contrats API](docs/API_CONTRACTS.md)
- [Modèle de données](docs/DATA_MODEL.md)
- [Collecte et formules](docs/METRICS_COLLECTION.md)
- [Sécurité](docs/SECURITY.md)
- [Stratégie de tests](docs/TESTING.md)
- [Règles de code](docs/CODE_RULES.md)
- [Modules d'administration (v3, à implémenter)](docs/SPEC_ADMINISTRATION.md)
