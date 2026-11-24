# Instructions Claude Code — Homelab Manager

Les instructions du projet sont dans `AGENTS.md`, fichier unique partagé par tous les assistants de code. Ne pas dupliquer son contenu ici.

@AGENTS.md

## Spécifique à Claude Code

- Répondre à l'utilisateur en français.
- Avant de déclarer une tâche terminée, lancer `scripts/verify.sh` (avec Node 22, la version de la CI) et rapporter son résultat réel ; ne pas enchaîner un commit après lui sans avoir lu son code de sortie.
- Lancer les audits exigés par `docs/CODE_RULES.md` § 2.1 pour les fichiers modifiés (`scripts/audit.sh`) et en rapporter chaque résultat dans les catégories du § 2.2 : réussi, problèmes détectés, outil indisponible, non exécuté.
- Ne pas installer un outil d'audit ni modifier une version de dépendance sans l'accord de l'utilisateur ; signaler l'outil manquant avec sa commande d'installation.
