# Renommage en gOps — inventaire et plan (en attente)

> **État : abandonné.** Le 26 novembre 2026, le projet a été renommé **MonoOps**, et non gOps. Décisions retenues : `monoops` en minuscules pour le module Go, le binaire du Manager, son compte, ses chemins et son unité ; `monoops-<composant>` pour les autres unités et binaires ; préfixe d'environnement `MONOOPS_` ; le mot « Manager » reste le nom du composant serveur ; coupure nette, sans migration, aucune installation n'étant en service. L'inventaire ci-dessous a servi de liste de contrôle.
>
> État d'origine : demandé le 23 novembre 2026, reporté à la demande de l'utilisateur. Inventaire fait sur le code à la livraison de la phase 1C.

**Objectif :** remplacer le nom « Homelab Manager » par « gOps » dans le code, le déploiement, l'interface et la documentation, avec une procédure de migration pour les installations existantes.

## Décisions à prendre

| Id | Question | Proposition |
|---|---|---|
| R-1 | Forme technique du nom | « gOps » à l'affichage ; `gops` en minuscules partout ailleurs (module Go, comptes, chemins, unités) ; préfixe d'environnement `GOPS_`. Les comptes Linux restent en minuscules. |
| R-2 | Nom des binaires | `gops-server` et `gops-docker-proxy`. Un binaire `gops` entrerait en conflit avec l'outil de diagnostic Go du même nom (`github.com/google/gops`), souvent installé par `go install`. |
| R-3 | Le mot « Manager » dans la documentation (« le Manager démarre… ») | À trancher : le garder comme nom du composant serveur, ou le remplacer par « gOps ». |
| R-4 | Transition des variables `HOMELAB_*` | À trancher : les accepter une version de plus avec un avertissement journalisé, ou couper net. |

## Inventaire

Occurrences dans les fichiers suivis : `homelab-manager` 497 lignes dans 168 fichiers, « Homelab Manager » 60 lignes dans 33 fichiers, `HOMELAB_` 57 lignes dans 12 fichiers.

| Zone | Emplacements | Effet sur une installation existante |
|---|---|---|
| Module Go | `module homelab-manager/backend` et 322 imports, dont `backend/ent` généré | Aucun |
| Variables d'environnement | `v.SetEnvPrefix("HOMELAB")` (`backend/cmd/manager/config.go`), `.env.example`, README, `scripts/dev.sh`, unités | `manager.env` à réécrire (voir R-4) |
| Systemd et comptes | `homelab-manager.service`, `homelab-docker-proxy.service` et `.socket` ; comptes `homelab-manager`, `homelab-docker-proxy` | Nouvelles unités, nouveaux comptes, propriété des fichiers à transférer |
| Chemins système | `/etc/homelab-manager`, `/var/lib/homelab-manager/homelab-manager.db`, `/opt/homelab-manager/frontend`, `/run/homelab-docker-proxy/docker.sock`, `/usr/local/bin/homelab-*` | Base et configuration à déplacer, service arrêté |
| Polkit | `deployments/polkit/50-homelab-manager-power.rules` (contrôle du compte) | Règle à réinstaller, sinon l'alimentation est refusée |
| Clé du coffre | `homelab-manager-secrets-key` (`config.go`, `ImportCredential=`) | Fichier chiffré à recréer sous le nouveau nom, sinon TOTP indisponible |
| Cookies | `homelab_session`, `homelab_login` | Une déconnexion de tous les utilisateurs |
| TOTP | émetteur `"Homelab Manager"` (`usecase/second_factor.go`) | Enrôlements existants valides ; ancien libellé dans les applications |
| Stockage navigateur | `homelab-manager-theme`, `-refresh`, `-username` | Préférences perdues une fois |
| Interface | `index.html`, `Logo`, page de connexion, fil d'Ariane du tableau de bord, Paramètres, Utilisateurs, `lib/power.ts` ; `package.json` | Aucun |
| Releases | `homelab-manager_<version>_linux_<arch>.tar.gz` (`scripts/release/build.sh`), `gh release download -R …/homelab-manager` | Nouveaux noms d'archives |
| Documentation | README (en-tête, badges, installation, exploitation), `docs/*.md`, `AGENTS.md`, `CLAUDE.md`, plans | Aucun |
| Dépôt GitHub | `alexandrebouttierdev/homelab-manager` | Renommage par l'utilisateur dans GitHub (l'ancien nom redirige) |

## Exécution, une fois les décisions prises

1. Un seul commit `refactor!:` (changement incompatible : la release suivante passe en majeure, `3.0.0`), Ent régénéré, `scripts/verify.sh` vert.
2. README : section « Migrer depuis Homelab Manager » (arrêt, comptes, déplacement de `/etc`, `/var/lib`, `/opt`, clé `systemd-creds`, règle polkit, `manager.env`, redémarrage), testée sur une base copiée.
3. Parcours Chromium complet contre le binaire renommé (connexion, double authentification, comptes, alimentation désactivée).
