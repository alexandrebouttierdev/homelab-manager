# Indiquer comment corriger une vulnérabilité — conception

Date : 25 novembre 2026. Validée par le propriétaire du projet le même jour.

## Intention

Le propriétaire voulait d'abord que l'utilisateur puisse corriger lui-même, s'il le souhaite, les vulnérabilités détectées par le module V. Après examen (la mise à jour de paquets est un code exécuté en root, D-1 l'avait écartée, elle ouvrirait un nouvel accès réseau), il a retenu le minimum utile : « au moins qu'on sache comment la résoudre ». L'interface indique donc la marche à suivre ; le Manager n'exécute rien.

## Objectifs

- Dans le détail d'une vulnérabilité, un bloc « Comment la corriger » adapté à son état de correction (Grype) :
  - **Corrigeable** (`fixed`) : la commande à lancer sur la machine, avec un bouton « Copier », la version visée et l'étape suivante : redémarrer la machine pour un paquet du noyau, sinon relancer les services qui utilisent le paquet.
  - **Non corrigée** (`not-fixed`) : aucune correction publiée ; lien vers l'avis de la distribution quand il est connu ; relancer une analyse plus tard.
  - **Ne sera pas corrigée** (`wont-fix`) : la distribution a jugé la correction inutile ou impossible pour cette version ; lien vers l'avis.
  - **Inconnu** : la source ne dit rien ; lien vers l'avis.
- En tête des constats, une commande unique qui met à jour tous les paquets ayant au moins une vulnérabilité corrigeable présente.

## Non-objectifs

- Aucune exécution par le Manager : pas d'unité systemd, pas de règle polkit, pas d'accès réseau. D-1 reste valable.
- Pas de changement de l'API ni du schéma : tout est calculé dans l'interface à partir des données existantes (`packageType`, `package`, `fixState`, `fixedVersions`, avis public).

## Anti-objectifs

- Une commande fausse ou dangereuse à copier dans un shell root. Un nom de paquet qui ne respecte pas le motif `^[A-Za-z0-9][A-Za-z0-9+._-]{0,127}$` ne produit aucune commande, seulement le conseil générique de mettre à jour le système.
- Présenter la correction comme certaine : le texte dit « devrait corriger » et invite à relancer une analyse pour vérifier.

## Commandes, selon `packageType`

| Type | Distributions | Un paquet | Plusieurs paquets |
|---|---|---|---|
| `deb` | Debian, Ubuntu | `sudo apt update && sudo apt install --only-upgrade <p>` | même commande, paquets séparés par des espaces |
| `apk` | Alpine | `sudo apk update && sudo apk upgrade <p>` | idem |
| `rpm` | Fedora, RHEL, Rocky, AlmaLinux | `sudo dnf upgrade --refresh <p>` | idem |
| autre | — | pas de commande, conseil générique | — |

Paquet du noyau (redémarrage nécessaire) : `linux-image-*` (deb), `kernel` et `kernel-*` (rpm), `linux-*` (apk). Sinon : relancer les services concernés (par exemple avec `needrestart` sur Debian) ou redémarrer la machine.

## Commande globale

Chargée par l'API existante : `GET /api/v1/vulnerabilities?state=present&fixable=true&limit=500`. Paquets dédupliqués et triés, regroupés par type (une commande par type présent). Si le total dépasse 500 constats, la commande est remplacée par la mise à jour complète du système (`apt upgrade`, `apk upgrade`, `dnf upgrade`), avec la raison. Sans constat corrigeable, le bloc n'apparaît pas.

## Approches considérées

1. **Instructions calculées dans l'interface** (retenue) : aucune modification du contrat ni du privilège, fonctions pures testées.
2. Instructions calculées par le serveur et ajoutées au DTO : contrat modifié sans gain, la logique est de la présentation.
3. Guide générique sans commande : moins utile, l'utilisateur doit retrouver le nom du paquet et la syntaxe.
4. Correction exécutée par le Manager (demande initiale) : écartée par le propriétaire au profit de l'option 1 ; elle exigerait une unité root exécutant les scripts des paquets et un accès aux miroirs.

## Laissé au développeur

- Emplacement exact des blocs dans `VulnerabilitiesPanel.tsx` et libellés précis.
- Le bouton « Copier » suit le modèle de `useBackupSSHKey.ts` (masqué sans `navigator.clipboard`, message de confirmation ou d'échec).
- Logique dans `frontend/src/lib/vulnerabilities.ts` (fonctions pures, testées) et `useVulnerabilitiesPanel.ts` ; le composant n'affiche que le modèle de vue.
- Documentation : SPEC_ADMINISTRATION.md § 8 ter.5, PRODUCT_SPEC.md si la liste des fonctions livrées le mentionne.
