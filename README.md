# Homelab Manager

Homelab Manager supervise une seule machine Linux : celle qui héberge le Manager. Le Backend Go collecte les métriques locales et découvre les conteneurs Docker ; PostgreSQL conserve les mesures et événements ; l'interface React est servie sur la même origine que l'API.

La spécification produit v2.0 est dans [docs/PRODUCT_SPEC.md](docs/PRODUCT_SPEC.md). Les règles de contribution sont dans [CLAUDE.md](CLAUDE.md).

## Architecture

- `backend/` : module Go unique, API REST Chi v5, Ent/PostgreSQL, collecte Linux et Docker.
- `frontend/` : React, TypeScript strict, Vite, React Router, TanStack Query, Zod, React Hook Form, Tailwind CSS et ECharts.
- `deployments/systemd/` : unités Manager et proxy Docker local en liste blanche.
- `docs/` : architecture, contrats API, données, sécurité, métriques et stratégie de tests.

Aucun Agent distant, enrôlement, protocole Manager-Agent, commande distante ou fonction d'administration des conteneurs n'est inclus. Docker Compose n'est pas requis pour la production.

## État fonctionnel et limites connues

Le code v2.0 est en place : authentification locale, collecteurs système/Docker, API protégée, historique PostgreSQL, événements de changement de conteneur, interface dashboard/hôte/containers/images/volumes/réseaux Docker/stockage/métriques/réseau/événements/paramètres, proxy Docker Unix filtré et unités systemd. L'interface utilise Tailwind CSS et des composants React internes ; les écrans sans support Backend restent explicitement marqués « à venir ».

Les limites actuelles sont explicites :

- Les historiques renvoient des échantillons bruts (maximum 2 000 points par requête). Agrégation par pas et curseurs ne sont pas encore implémentés ; les paramètres correspondants sont rejetés.
- Les listes sont bornées et ne fournissent pas encore de pagination.
- Les tests PostgreSQL d'intégration nécessitent une base réelle dédiée et ne s'exécutent pas sans cette configuration.
- L'installation système (comptes, permissions et groupes) doit être réalisée par l'administrateur avant d'activer les unités systemd.

La migration `002_single_host.sql` supprime les tables métier de l'ancienne architecture multi-Agents ainsi que leurs données. **Sauvegarder PostgreSQL avant de mettre à niveau** ; cette suppression est intentionnelle et irréversible par la migration.

## Prérequis

- Go `1.26.10` ou plus récent.
- Node.js 22 et npm.
- PostgreSQL local dédié.
- Docker Engine optionnel ; la collecte Docker passe par `homelab-docker-proxy.socket`.

## Développement

1. Démarrer PostgreSQL et créer la base `homelab_manager` avec un rôle dédié.
2. Copier [.env.example](.env.example) vers `.env`, puis définir `HOMELAB_ADMIN_PASSWORD` avec un secret fort.
3. Installer les paquets Frontend :

   ```bash
   npm --prefix frontend ci
   ```

4. Lancer le Backend et Vite :

   ```bash
   scripts/dev.sh
   ```

   Vite écoute sur `http://127.0.0.1:5173` et transmet `/api` au Manager sur le port 8080. Les migrations versionnées sont appliquées au démarrage.

Pour lancer les processus séparément :

```bash
set -a
source .env
set +a
go -C backend run ./cmd/manager
npm --prefix frontend run dev -- --host 127.0.0.1
```

Sans PostgreSQL, le Manager répond sur `/health` et rapporte son état dégradé sur `/api/v1/health`, mais l'authentification et la supervision persistée ne sont pas disponibles.
Si PostgreSQL ou les migrations sont indisponibles au démarrage, corriger la cause puis redémarrer le Manager pour initialiser les adaptateurs de persistance.

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
- `HOMELAB_DATABASE_URL` : URL PostgreSQL ; TLS est à configurer pour les connexions non locales.
- `HOMELAB_ADMIN_USERNAME` / `HOMELAB_ADMIN_PASSWORD` : création initiale de l'administrateur ; le mot de passe fait 12 à 1 024 octets et est haché avec Argon2id.
- `HOMELAB_TLS_CERT_FILE` / `HOMELAB_TLS_KEY_FILE` : active HTTPS et les cookies `Secure`.
- `HOMELAB_SESSION_SECURE` : force l'attribut `Secure` si TLS est terminé par un proxy.
- `HOMELAB_SYSTEM_COLLECT_INTERVAL_SECONDS` : collecte système (5–300 s, défaut 10).
- `HOMELAB_DOCKER_COLLECT_INTERVAL_SECONDS` : statistiques Docker (5–300 s, défaut 15).
- `HOMELAB_DOCKER_DISCOVERY_INTERVAL_SECONDS` : inventaire Docker (10–600 s, défaut 30).
- `HOMELAB_METRICS_RETENTION_DAYS` : rétention des métriques (1–365 jours, défaut 30).
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

Installer les binaires sous `/usr/local/bin`, le contenu de `frontend/dist` sous `/opt/homelab-manager/frontend/dist`, et fournir `/etc/homelab-manager/manager.env` en mode `0600`. Créer les comptes système `homelab-manager` et `homelab-docker-proxy` ainsi que le groupe `homelab-manager` ; ajouter uniquement le compte proxy au groupe Docker. Le Manager doit appartenir au groupe `homelab-manager` pour accéder au socket proxy.

Installer et activer les unités `homelab-docker-proxy.socket` et `homelab-manager.service`. Le socket proxy écoute seulement sur Unix, autorise les routes GET nécessaires, filtre `Env` et `Labels`, borne la taille des réponses à 8 MiB et la concurrence à 16 requêtes. Vérifier les permissions effectives avant le démarrage.

Le Manager applique les migrations au démarrage. Sauvegarder la base avant toute mise à niveau et contrôler les journaux systemd après installation.

## Documentation

- [Spécification produit](docs/PRODUCT_SPEC.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Contrats API](docs/API_CONTRACTS.md)
- [Modèle de données](docs/DATA_MODEL.md)
- [Collecte et formules](docs/METRICS_COLLECTION.md)
- [Sécurité](docs/SECURITY.md)
- [Stratégie de tests](docs/TESTING.md)
