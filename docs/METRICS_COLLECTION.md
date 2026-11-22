# Collecte et formules des métriques

Version 2.1 — 22 novembre 2026

## 1. Cycle de collecte

Le Backend démarre en arrière-plan des collecteurs locaux indépendants : système Linux, découverte/événements Docker et statistiques Docker. Ils ne dépendent pas de requêtes HTTP ni d'une page ouverte. Chaque lot porte son horodatage UTC, son statut et la fraîcheur de la dernière réussite. Les erreurs ponctuelles ne sont pas transformées en mesures nulles ou en zéros.

| Cycle | Paramètre | Bornes | Défaut |
|---|---|---|---|
| Collecte système | `system_collect_interval_seconds` | 5–300 s | 10 s |
| Statistiques Docker | `docker_collect_interval_seconds` | 5–300 s | 15 s |
| Découverte Docker (inventaire + événements) | `docker_discovery_interval_seconds` | 10–600 s | 30 s |
| Rétention | `metrics_retention_days`, puis interface | 1–365 jours | 7 jours |
| Purge | fixe | — | au démarrage puis toutes les heures |

Chaque cycle s'exécute une première fois au démarrage, puis à son intervalle, dans sa propre goroutine. Un cycle dispose d'un délai maximal de `min(max(intervalle, 10 s), 30 s)` ; au-delà, son contexte est annulé et l'erreur est journalisée. Une collecte système échouée ne bloque pas Docker, et inversement. Rien n'est mis en file d'attente en mémoire : un cycle manqué est perdu, le suivant repart de l'état courant.

Les écritures SQLite se font par lots de 500 échantillons. La purge supprime par lots de 1 000 lignes (métriques puis événements) jusqu'à épuisement des données expirées ; si son délai expire, la purge suivante reprend.

## 2. Métriques système Linux

Toutes les métriques sont facultatives selon les interfaces et permissions disponibles.

### CPU et charge

Lire les compteurs de `/proc/stat` (global et par cœur) à chaque cycle et calculer l'écart avec le cycle précédent :

```text
total = user + nice + system + idle + iowait + irq + softirq + steal
idle  = idle + iowait
CPU utilisé (%) = 100 × (Δtotal − Δidle) / Δtotal
```

`guest` et `guest_nice` sont déjà inclus par le noyau dans `user` et `nice` : ils ne sont pas ajoutés une seconde fois. L'attente d'E/S n'est pas comptée comme du temps actif. Le premier cycle après le démarrage ne produit pas de `cpu.percent` (pas de référence) ; si `Δtotal ≤ 0` ou si les compteurs décroissent, le point est absent. Le pourcentage couvre donc tout l'intervalle de collecte, pas un échantillon instantané. Charge 1/5/15 minutes depuis `/proc/loadavg` ; `cpu.cores` = nombre de CPU logiques.

### Mémoire et swap

Lire `/proc/meminfo` :

```text
RAM utilisée = MemTotal - MemAvailable
Swap utilisée = SwapTotal - SwapFree
```

`memory.cached` vaut `Cached + Buffers` ; il est informatif et n'est pas retranché une seconde fois de la mémoire utilisée. Si `MemAvailable` manque ou dépasse `MemTotal`, `memory.used` et `memory.available` sont absents : aucune équivalence n'est inventée.

### Stockage et activité bloc

Capacité des systèmes de fichiers depuis `statfs` (ou interface équivalente) :

```text
utilisé = capacité totale - capacité disponible
utilisation (%) = 100 × utilisé / capacité totale
```

L'espace disponible est `f_bavail × f_bsize` (espace accessible aux comptes non privilégiés), ce qui exclut la réserve root : `filesystem.percent` correspond à ce que voit `df`. Seules les partitions physiques montées sont collectées ; un point de montage illisible est ignoré. Chaque échantillon porte `mountpoint` et `device` : les sous-volumes btrfs et les montages *bind* exposent le même volume sous plusieurs points de montage, et toute somme (capacité totale, espace utilisé) doit compter chaque `device` une seule fois. L'espace « utilisé » vaut `total − disponible` : sur btrfs, il inclut les métadonnées et dépasse légèrement la colonne « Utilisé » de `df`. Lire `/proc/diskstats` pour les compteurs bloc lorsque présents :

```text
read_iops = Δlectures terminées / Δtemps (secondes)
write_iops = Δécritures terminées / Δtemps (secondes)
latence moyenne lecture (ms) = Δtemps lecture (ms) / Δlectures terminées
```

Équivalent pour l'écriture. Ces latences sont des moyennes agrégées noyau, non une latence de chaque requête. Si compteurs ou intervalle invalides, laisser la métrique indisponible. Les compteurs système bloc ne sont pas attribués à un conteneur.

### Réseau

Lire les compteurs d'interfaces (bibliothèque Go adaptée ou `/proc/net/dev`) et calculer les deltas entre deux collectes :

```text
débit entrant (octets/s) = Δoctets reçus / Δtemps
débit sortant (octets/s) = Δoctets envoyés / Δtemps
```

Les octets, paquets, erreurs et drops sont également conservés comme compteurs. Si un compteur décroît (remise à zéro, interface recréée), le débit de cet intervalle est omis et la nouvelle valeur sert de référence.

Les adresses IP et MAC sont de l'inventaire, pas des mesures. Elles sont aujourd'hui publiées comme échantillon `network.interface` (valeur `1`, labels `name`, `address`, `mac`, une ligne par adresse) dans chaque cycle, afin que l'interface puisse les afficher sans table dédiée. C'est la seule exception au principe « une métrique = une mesure ».

### Températures et système

Lire les capteurs exposés par `/sys/class/thermal`, `hwmon` ou sources locales équivalentes. Les disques ne sont affichés que si leur température est réellement exposée par une interface autorisée ; aucun recours à une commande `smartctl` par cycle n'est supposé. Capteur absent/inaccessible = non disponible.

Hostname, distribution/version, noyau, uptime et date de démarrage proviennent d'interfaces locales stables. Ne jamais exécuter des commandes shell répétées pour les obtenir.

## 3. Métriques Docker

Utiliser les données de `stats` Docker Engine API pour chaque conteneur. L'adaptateur gère les champs de cache courants de cgroups v1 et v2 ; les métriques indisponibles sont omises plutôt que fabriquées.

### CPU

Pour deux lectures Docker, `cpu_delta = cpu_stats.cpu_usage.total_usage - precpu_stats.cpu_usage.total_usage`, `system_delta = cpu_stats.system_cpu_usage - precpu_stats.system_cpu_usage`. `online_cpus` vient de l'API ; si absent ou nul, utiliser le nombre d'entrées `percpu_usage` disponibles.

```text
CPU (%) = 100 × cpu_delta / system_delta × online_cpus
```

Si l'un des deltas est nul ou négatif, point absent. Il s'agit du pourcentage de capacité CPU de l'hôte, qui peut dépasser 100 % pour un conteneur multi-cœur ; ce n'est pas le pourcentage du quota configuré.

### Mémoire

Docker fournit `memory_stats.usage`, `limit` et statistiques de cache :

```text
usage_effectif = max(usage_brut - cache_inactif, 0)
mémoire disponible = max(limite - usage_effectif, 0)
mémoire (%) = 100 × usage_effectif / limite
```

`cache_inactif` : `total_inactive_file` sous cgroups v1, `inactive_file` sous v2. Si ces champs manquent, `usage_brut` est utilisé tel quel. Si `usage_brut` vaut 0, aucune métrique mémoire n'est émise. Une limite absente ou nulle supprime `memory.limit_bytes`, `memory.available_bytes` et `memory.percent`, jamais remplacés par un taux inventé. Sans limite configurée, Docker renvoie la mémoire de l'hôte comme limite. `memory.usage_bytes` n'est pas une mémoire RSS pure.

### Réseau et stockage bloc

Conserver les compteurs RX/TX bytes, packets, errors et dropped par interface retournée par Docker. Calculer les taux RX/TX bytes depuis deux lectures successives ; les taux utilisent le temps réel écoulé, pas une fréquence supposée. Si un compteur baisse, omettre le taux de cet intervalle.

Les bytes de lecture/écriture viennent des entrées read/write de `blkio_stats.io_service_bytes_recursive` lorsqu'elles existent. Docker Stats ne garantit pas la disponibilité de ces champs pour tous runtimes/cgroups. IOPS et latence d'E/S par conteneur ne sont pas déclarées disponibles à partir de ces seuls compteurs.

### Autres informations

- Nombre de processus depuis `pids_stats.current` si fourni.
- Nombre de redémarrages depuis l'inspection Docker. `restart.count` porte le même horodatage que les statistiques du conteneur (même cycle) ; pour un conteneur arrêté, il porte l'heure du cycle.
- Uptime calculé depuis `StartedAt` uniquement si l'état est running.
- État, ports, réseaux, volumes et politique de redémarrage depuis les réponses d'inspection/listing, avec secrets filtrés.
- Un conteneur qui disparaît entre la liste et son inspection (réponse 404) est ignoré pour ce cycle ; toute autre erreur fait échouer la découverte, sans modifier l'inventaire connu.

## 4. Historique, fraîcheur et rétention

Échantillons horodatés en UTC, écrits par lots dans SQLite. Par défaut, 7 jours de données brutes et d'événements sont conservés. La valeur de la configuration (`metrics_retention_days`, 1–365) est la valeur par défaut ; l'utilisateur la remplace depuis Paramètres (API_CONTRACTS.md § 4.4), avec effet immédiat et purge aussitôt. La purge horaire supprime tout ce qui est plus ancien que la rétention, par lots, sur l'index temporel. Les historiques s'agrègent à la lecture (1 min, 5 min, 1 h) ; aucun agrégat n'est stocké.

**Volume attendu.** Mesuré sur une machine à 8 cœurs, 4 systèmes de fichiers, 5 disques, 2 interfaces et 20 capteurs : environ 100 échantillons par cycle, soit **≈ 870 000 lignes par jour à l'intervalle par défaut de 10 s, ≈ 340 octets par ligne index compris, ≈ 300 Mo par jour**. La taille stable de la base est d'environ `300 Mo × rétention en jours` (≈ 2 Go pour les 7 jours par défaut, ≈ 9 Go pour 30 jours), plus environ 10 à 15 échantillons par conteneur en cours d'exécution tous les 15 s. Pour réduire le volume : allonger `system_collect_interval_seconds` (le volume est inversement proportionnel) ou réduire la rétention. L'interface affiche l'estimation de taille correspondant à la rétention saisie.

Les vues courantes affichent `status`/`lastSuccessAt` du collecteur selon API_CONTRACTS.md § 4.1 (`starting`, `ok`, `stale` au-delà de 3 intervalles sans succès, `unavailable`). Une valeur zéro est une mesure réelle égale à zéro, pas une donnée inconnue.
