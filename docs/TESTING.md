# Stratégie de tests — Homelab Manager

Version 2.1 — 22 novembre 2026

## Backend Go

### Collecte système
- Tester le parsing avec fixtures `/proc` et `/sys`, sans dépendre des ressources de la machine de CI.
- Tester compteurs absents, données malformées, permissions insuffisantes, capteurs indisponibles et annulation de contexte.
- Tester les calculs de deltas CPU, mémoire, stockage et débit réseau, y compris compteurs réinitialisés et intervalles nuls.
- Ne jamais exiger une température matérielle réelle dans les tests.

### Docker
- Définir un port d'adaptateur simulable ; les tests unitaires n'exigent pas de daemon Docker.
- Tester conteneurs arrêtés, nouveaux/supprimés/redémarrés, erreurs temporaires, cgroups v1/v2 et champs non disponibles.
- Tester formules CPU/mémoire et agrégation réseau/bloc avec fixtures de réponses Engine API.
- Tester que routes/méthodes non autorisées sont refusées par le proxy et que données sensibles (comme Env) ne sont pas exposées.
- Tests d'intégration optionnels contre un daemon local dédié, sans modifier des conteneurs de l'hôte de développeur.

### Usecases, persistance et HTTP
- Tester planification, indépendance des collecteurs, transitions et rétention avec horloge contrôlée et ports simulés.
- Tests SQLite réels (fichier temporaire, mêmes pragmas que la production) pour migrations, transactions, purge, requêtes temporelles et accès concurrents.
- `httptest` pour routes Chi, DTO, auth, limites, erreur, bornes temporelles et contrôle d'accès.
- Tester annulation et arrêt ordonné de chaque goroutine de collecte.

Commandes du module cible :
```bash
gofmt -w .
go vet ./...
go test ./...
go test -race ./...
```

Les tests d'intégration SQLite (`internal/adapters/sqlite`) s'exécutent avec `go test ./...` sans configuration : chaque test ouvre une base neuve dans `t.TempDir()` via le même `Open` que le Manager. Pour les lancer seuls :

```bash
go test ./internal/adapters/sqlite -count=1 -v
```

## Frontend React

- Vitest + React Testing Library pour dashboard, navigation, détails machine/conteneur et états loading/empty/error/stale.
- Tester graphiques avec données/vides/erreurs, fenêtres temporelles, labels et fraîcheur.
- Tester schémas Zod, formulaires, authentification, client REST et paramètres.
- Vérifier interactions clavier, labels et responsive.
- Aucun test ne doit afficher des données simulées comme des données réelles.

Scripts de release : `scripts/release/release_test.sh` (lancé par `scripts/verify.sh`) vérifie dans un dépôt Git jetable le calcul de version (sans tag, correctif, `feat`, changement cassant, commit hors convention, HEAD déjà publié) et le groupement des notes. La construction des archives (`scripts/release/build.sh`) se vérifie en l'exécutant : binaires statiques `amd64` et `arm64`, version présente dans les journaux et l'interface.

Commandes :
```bash
npm run format:check
npm run lint
npm run typecheck
npm run test
npm run build
```

## Parcours transverses

- Démarrage sans Docker puis récupération après indisponibilité du daemon.
- Détection à chaud de création, arrêt, redémarrage et suppression d'un conteneur.
- Collecte système poursuivie si Docker est indisponible, et inversement.
- Historique, bornes temporelles et rétention.
- Authentification, contrôle d'origine, sessions révoquées et accès proxy strictement local.
- Démarrage/arrêt natif Debian 13 via systemd et vérification des permissions des sockets.

### Couverture actuelle

Les sections précédentes fixent les exigences. Aujourd'hui couverts automatiquement : formules CPU (deltas `/proc/stat`), débits réseau/disque et remises à zéro, formules CPU/mémoire Docker, découverte Docker avec conteneur disparu (API simulée), liste blanche du proxy, usecases (indépendance des collecteurs, arrêt, état `stale`, horodatage de `restart.count`, purge complète d'un arriéré, rétention choisie dans l'interface et purge immédiate, anti-énumération à la connexion), sessions et politique de sécurité (bornes de la politique, préfixes IPv4/IPv6, verrouillage après 5 échecs avec horloge fixe, inactivité et écriture de la dernière activité une fois par minute, règles de révocation, auto-exclusion refusée, refus réseau sauf `/health`, audit des refus, filtres et export CSV/JSON du journal, neutralisation des formules CSV, migration `004` sur une base existante), journal système (validation des filtres, curseurs et unités, masquage, recherche de texte, synthèse par tranches, construction des arguments de `journalctl`, décodage JSON des octets et des valeurs multiples, lignes trop longues, environnement non transmis, délai et arrêt anticipé avec un faux `journalctl`, intégration contre le vrai `journalctl` sur des fichiers fabriqués par `systemd-journal-remote` : pagination dans les deux sens, filtres, intervalle avec curseur, démarrages, unités, occupation disque ; pagination et recherche bornée du usecase, cache de la synthèse, export ; routes HTTP pour tous les rôles, analyse stricte de la chaîne de requête), services système (adaptateur D-Bus contre un faux systemd exporté par godbus sur un bus privé lancé par `dbus-daemon` — ignoré s'il manque, installé en CI : inventaire trié « en échec d'abord » et borné, valeurs absentes `UINT64_MAX`, 0 et PID 0 rendues nulles, minuterie et socket, unité disparue entre deux appels, détail avec `ExecStart`, dépendances bornées à 100 et limites « illimité » distinctes de « inconnu », unité inconnue, bus sans systemd, nom invalide refusé sans connexion, aucun appel autre que `ListUnits`, `GetUnit` et la lecture de propriétés ; transitions d'échec pures et table `failed_units` sur SQLite réel : ensemble et événements dans une même transaction, aucun doublon après un redémarrage, ensemble conservé après la purge des événements ; usecase : échec signalé une fois puis retour à l'état actif, ensemble relu après un redémarrage, écriture en échec reprise au cycle suivant, statuts `pending`, `ok`, `error` (inventaire précédent conservé) et `unavailable`, délai par cycle, collecte immédiate et arrêt par contexte, détail limité au dernier inventaire avec 50 entrées du journal de l'unité et sans journal disponible), second facteur (vecteurs RFC 6238 en 8 et 6 chiffres, fenêtre ±1 période, anti-rejeu, codes de secours normalisés et à usage unique, chiffrement AES-256-GCM avec données associées et clé invalide, matrice QR, enrôlement en attente puis confirmé, connexion en deux étapes et verrouillage par codes faux, défi expiré ou épuisé, codes de secours sans clé, obligation des administrateurs dans la connexion et la session, politique refusée à un administrateur sans facteur, parcours HTTP complet sur SQLite réel, migration `005`), actions d'alimentation (permission, confirmation par le nom d'hôte, audit écrit avant l'action, refus polkit classé, demande unique, désactivation par défaut, parcours HTTP avec un faux contrôleur logind), comptes et rôles (réinitialisation par un administrateur, changement imposé du mot de passe par défaut à la création et au démarrage, politique des rôles, validation des comptes, protection du dernier administrateur, révocation des sessions à la désactivation, refus d'un compte désactivé, parcours HTTP complet création → mot de passe imposé → refus par rôle → désactivation → suppression sur SQLite réel, migration `003` sur une base existante), routes HTTP (santé, SPA, 404/405 JSON, contrôle d'origine, validation des paramètres), intégration SQLite (voir plus bas), dont l'agrégation (moyenne, min, max, nombre par intervalle), le filtre par label, le refus `TOO_MANY_POINTS` et le stockage de la rétention avec son audit, configuration, client REST, schémas Zod (connexion, mot de passe, rétention) et estimation de taille du Frontend, pages de connexion et dashboard, client des comptes, schéma de compte, libellés de rôles, redirection vers le changement de mot de passe imposé et vers l'enrôlement imposé, page Utilisateurs, deuxième étape de connexion (code d'application puis code de secours), carte « Double authentification » (QR code, codes de secours affichés une fois, clé absente), schémas et tracé du QR code, page « Journaux système » (liste, inspecteur masqué, entrées plus anciennes, suivi en direct, filtres et lien d'export), client du journal (curseurs encodés), retrait des séquences de terminal, fusion des entrées suivies, synthèse par priorité, schéma de recherche et d'intervalle.

Non couverts automatiquement, à vérifier manuellement avant une version : parcours transverses ci-dessus, démarrage systemd réel et permissions des sockets, redémarrage et extinction réels par logind et polkit (machine virtuelle jetable uniquement), accessibilité clavier et affichage mobile.

Les tests doivent valider la cible mono-machine ; aucun test Agent distant, enrôlement ou protocole Manager-Agent ne fait partie de la spécification.

L'intégration SQLite vérifie la réouverture sans réappliquer les migrations, les pragmas effectifs (WAL, clés étrangères, `busy_timeout`), le mode `0600` du fichier, l'authentification/persistance des sessions, l'hôte, les métriques, la réconciliation des conteneurs, les événements, la purge, le dernier cycle complet (`LatestMetrics`), l'ordre chronologique des plages quel que soit le fuseau des dates reçues, et les écritures/lectures concurrentes. Les tests unitaires vérifient également les formules Docker, les deltas système, la liste blanche du proxy et les routes HTTP de base.
