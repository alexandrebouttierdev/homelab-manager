# Collecte et formules des métriques

Version 2.0 — 21 novembre 2026

## 1. Cycle de collecte

Le Backend démarre en arrière-plan des collecteurs locaux indépendants : système Linux, découverte/événements Docker et statistiques Docker. Ils ne dépendent pas de requêtes HTTP ni d'une page ouverte. Chaque lot porte son horodatage UTC, son statut et la fraîcheur de la dernière réussite. Les erreurs ponctuelles ne sont pas transformées en mesures nulles ou en zéros.

Les fréquences sont configurables avec les limites cibles suivantes : système 5–300 secondes (défaut 10), stats Docker 5–300 secondes (défaut 15), découverte complète 10–600 secondes (défaut 30), rétention 1–365 jours (défaut 30). La purge s'exécute quotidiennement par petits lots ; ces intervalles sont indépendants.

Une collecte système échouée ne bloque pas Docker, et inversement. Les écritures PostgreSQL se font par lots avec timeouts. En cas d'indisponibilité, log structuré, statut stale et nouvelle tentative temporisée ; ne pas accumuler une file mémoire non bornée.

## 2. Métriques système Linux

Toutes les métriques sont facultatives selon les interfaces et permissions disponibles.

### CPU et charge

Lire les compteurs `/proc/stat`. Pour un intervalle entre deux échantillons, `total` est la somme des compteurs CPU et `idle` désigne la somme des deltas `idle` et `iowait` :

```text
CPU utilisé (%) = 100 × (Δtotal - Δidle) / Δtotal
```

La même formule s'applique par cœur. Pour `/proc/stat`, `idle` désigne la somme des deltas `idle` et `iowait`, afin que l'attente I/O ne soit pas comptée comme du temps CPU actif. Si `Δtotal <= 0`, le point est absent. Charge système 1/5/15 minutes depuis `/proc/loadavg`, nombre de cœurs logiques depuis les interfaces locales.

### Mémoire et swap

Lire `/proc/meminfo` :

```text
RAM utilisée = MemTotal - MemAvailable
Swap utilisée = SwapTotal - SwapFree
```

`Cached`, `Buffers` et `SReclaimable` sont exposés séparément si disponibles et ne sont pas additionnés une seconde fois à la mémoire utilisée. Si `MemAvailable` manque, ne pas inventer une équivalence silencieuse.

### Stockage et activité bloc

Capacité des systèmes de fichiers depuis `statfs` (ou interface équivalente) :

```text
utilisé = capacité totale - capacité disponible
utilisation (%) = 100 × utilisé / capacité totale
```

Disponibilité est calculée selon l'espace disponible à l'utilisateur du service. Lire `/proc/diskstats` pour les compteurs bloc lorsque présents :

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

Les octets, paquets, erreurs et drops sont également conservés comme compteurs. Si un compteur décroît (reset/interface recréée), commencer une nouvelle base et omettre le débit de cet intervalle. IP/MAC sont des informations d'inventaire volatiles, non des métriques.

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

`cache_inactif` : `total_inactive_file` sous cgroups v1, `inactive_file` sous v2. Si ces champs manquent, utiliser `usage_brut` et indiquer la méthode/champ dans les labels ou la fraîcheur. Une limite absente, nulle ou non finie donne un pourcentage indisponible (`null`), jamais un taux inventé. `usage` ne doit pas être décrit comme mémoire RSS pure.

### Réseau et stockage bloc

Conserver les compteurs RX/TX bytes, packets, errors et dropped par interface retournée par Docker. Calculer les taux RX/TX bytes depuis deux lectures successives ; les taux utilisent le temps réel écoulé, pas une fréquence supposée. Si un compteur baisse, omettre le taux de cet intervalle.

Les bytes de lecture/écriture viennent des entrées read/write de `blkio_stats.io_service_bytes_recursive` lorsqu'elles existent. Docker Stats ne garantit pas la disponibilité de ces champs pour tous runtimes/cgroups. IOPS et latence d'E/S par conteneur ne sont pas déclarées disponibles à partir de ces seuls compteurs.

### Autres informations

- Nombre de processus depuis `pids_stats.current` si fourni.
- Nombre de redémarrages depuis l'inspection Docker, avec horodatage de l'observation.
- Uptime calculé depuis `StartedAt` uniquement si l'état est running.
- État, ports, réseaux, volumes et politique de redémarrage depuis les réponses d'inspection/listing, avec secrets filtrés.

## 4. Historique, fraîcheur et rétention

Échantillons horodatés UTC, écrits par lots dans PostgreSQL. Valeur par défaut : conserver 30 jours de données brutes et d'événements, configurable entre 1 et 365 jours. Purger quotidiennement en petits lots sur index temporel. Les endpoints imposent bornes temporelles et plafonds de points ; les historiques agrégés et curseurs ne sont pas encore disponibles.

`lastSuccessAt`, statut `ok/stale/unavailable` et âge de la mesure doivent accompagner les vues courantes. Une valeur zéro signifie une mesure réelle égale à zéro, pas une donnée inconnue.
