# Indiquer comment corriger une vulnérabilité — plan d'implémentation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** afficher, pour chaque vulnérabilité et pour l'ensemble des paquets corrigeables, la marche à suivre pour la corriger, sans que le Manager n'exécute rien.

**Architecture:** fonctions pures dans `frontend/src/lib/vulnerabilities.ts` (commandes, état, conseils) ; le hook `useVulnerabilitiesPanel.ts` charge les constats corrigeables et gère la copie ; `VulnerabilitiesPanel.tsx` n'affiche que le modèle de vue. Aucun changement du backend ni de l'API.

**Tech Stack:** React, TypeScript strict, TanStack Query, Vitest + React Testing Library.

**Spec:** `docs/superpowers/specs/2026-11-25-vulnerabilites-correction-design.md`

## Global Constraints

- Code et commits en anglais, interface et documentation en français, Conventional Commits.
- Aucun code métier dans les `.tsx` (CODE_RULES § 4.3) ; toute fonction de `src/lib/` a des tests (§ 4.6).
- Motif de nom de paquet accepté dans une commande : `^[A-Za-z0-9][A-Za-z0-9+._-]{0,127}$`.
- Commandes : `deb` → `sudo apt update && sudo apt install --only-upgrade <p…>` ; `apk` → `sudo apk update && sudo apk upgrade <p…>` ; `rpm` → `sudo dnf upgrade --refresh <p…>`.
- Mise à jour complète (repli) : `deb` → `sudo apt update && sudo apt upgrade` ; `apk` → `sudo apk update && sudo apk upgrade` ; `rpm` → `sudo dnf upgrade --refresh`.
- Paquets du noyau : `linux-image-*` (deb), `kernel` et `kernel-*` (rpm), `linux-*` (apk).
- Commande globale : `listFindings({ state: 'present', severities: [], fixableOnly: true, search: '', limit: 500, offset: 0 })`.
- `scripts/verify.sh` passe avant chaque commit.

## Review Focus

- Nom de paquet hors motif (espace, `;`, `$(…)`) : aucune commande, seulement le conseil générique. Test dans la tâche 1.
- `packageType` inconnu (`binary`, `''`) : conseil générique, jamais une commande apt par défaut. Test dans la tâche 1.
- Même paquet touché par plusieurs vulnérabilités : il n'apparaît qu'une fois dans la commande globale. Test dans la tâche 1.
- Plus de 500 constats corrigeables (`total > findings.length`) : repli sur la mise à jour complète, avec la raison. Test dans la tâche 1.
- `navigator.clipboard` absent (HTTP sans TLS) : bouton « Copier » masqué, la commande reste sélectionnable. Test dans la tâche 2.

---

### Task 1: Fonctions pures de correction

**Files:**
- Modify: `frontend/src/lib/vulnerabilities.ts`
- Test: `frontend/src/lib/vulnerabilities.test.ts`

**Interfaces:**
- Produces :
  - `upgradeCommand(packageType: string, packages: string[]): string | null` — `null` si le type est inconnu, la liste vide ou un nom hors motif ; paquets dédupliqués et triés.
  - `fullUpgradeCommand(packageType: string): string | null`
  - `isKernelPackage(packageType: string, name: string): boolean`
  - `type Remediation = { kind: 'command' | 'generic' | 'not-fixed' | 'wont-fix' | 'unknown'; summary: string; command: string | null; target: string | null; followUp: string | null }`
  - `remediationFor(finding: Pick<Finding, 'package' | 'packageType' | 'fixState' | 'fixedVersions'>): Remediation`
  - `type FixAllCommand = { packageType: string; label: string; command: string; packages: number; complete: boolean; note: string | null }`
  - `fixAllCommands(page: Pick<FindingPage, 'findings' | 'total'>): FixAllCommand[]` — un élément par `packageType` reconnu ; `complete: false` et `command` = mise à jour complète si `total > findings.length`.
  - `FindingDetail` reçoit un champ `remediation: Remediation` rempli par `toFindingDetail`.

- [ ] **Step 1: Écrire les tests qui échouent** (`describe('remediation')`) :
  - `upgradeCommand('deb', ['openssl', 'libssl3', 'openssl'])` → `'sudo apt update && sudo apt install --only-upgrade libssl3 openssl'`
  - `upgradeCommand('apk', ['musl'])` → `'sudo apk update && sudo apk upgrade musl'` ; `upgradeCommand('rpm', ['openssl-libs'])` → `'sudo dnf upgrade --refresh openssl-libs'`
  - `upgradeCommand('deb', ['openssl; rm -rf /'])`, `upgradeCommand('deb', ['$(id)'])`, `upgradeCommand('binary', ['x'])`, `upgradeCommand('deb', [])` → `null`
  - `isKernelPackage('deb', 'linux-image-6.12.0-amd64')` → `true` ; `('deb', 'linux-libc-dev')` → `false` ; `('rpm', 'kernel-core')` et `('rpm', 'kernel')` → `true` ; `('apk', 'linux-lts')` → `true`
  - `remediationFor({ package: 'openssl', packageType: 'deb', fixState: 'fixed', fixedVersions: ['3.5.4-1'] })` → `kind: 'command'`, `command` celle de `upgradeCommand`, `target: '3.5.4-1'`, `followUp` contient « services »
  - même cas avec `package: 'linux-image-amd64'` → `followUp` contient « Redémarrer la machine »
  - `fixState: 'fixed'` avec `packageType: 'binary'` → `kind: 'generic'`, `command: null`
  - `fixState: 'not-fixed'` → `kind: 'not-fixed'`, `command: null` ; idem `wont-fix` et `unknown`
  - `fixAllCommands({ findings: [deb openssl, deb openssl, deb curl], total: 3 })` → un élément, `command` `'… --only-upgrade curl openssl'`, `packages: 2`, `complete: true`
  - `fixAllCommands({ findings: [deb openssl], total: 620 })` → `complete: false`, `command: 'sudo apt update && sudo apt upgrade'`, `note` contient « 620 »
  - `fixAllCommands({ findings: [], total: 0 })` → `[]`
  - `toFindingDetail(finding).remediation.kind` vaut `'command'` pour un constat `fixed` de type `deb`

- [ ] **Step 2: Lancer les tests et constater l'échec**

Run: `cd frontend && npx vitest run src/lib/vulnerabilities.test.ts`
Expected: FAIL (fonctions non exportées)

- [ ] **Step 3: Implémenter les fonctions** dans `frontend/src/lib/vulnerabilities.ts`. Textes français : `summary` pour `command` « Mettre à jour le paquet devrait corriger cette vulnérabilité. » ; `not-fixed` « Aucune correction publiée pour l’instant : relancer une analyse plus tard. » ; `wont-fix` « La distribution ne prévoit pas de correction pour cette version. » ; `unknown` et `generic` « Mettre à jour les paquets du système, puis relancer une analyse. » ; `followUp` noyau « Redémarrer la machine pour charger le nouveau noyau. », sinon « Relancer les services qui utilisent ce paquet (needrestart sur Debian), ou redémarrer la machine. ». `label` de `FixAllCommand` : `deb` « Debian / Ubuntu (apt) », `apk` « Alpine (apk) », `rpm` « Fedora / RHEL (dnf) ».

- [ ] **Step 4: Lancer les tests et constater la réussite**

Run: `cd frontend && npx vitest run src/lib/vulnerabilities.test.ts`
Expected: PASS

- [ ] **Step 5: Commit** — `feat(vulnerabilities): compute how to fix a finding`

### Task 2: Affichage, copie et documentation

**Files:**
- Modify: `frontend/src/features/host/useVulnerabilitiesPanel.ts`, `frontend/src/features/host/VulnerabilitiesPanel.tsx`
- Test: `frontend/src/features/host/VulnerabilitiesPanel.test.tsx`
- Modify: `docs/SPEC_ADMINISTRATION.md` (§ 8 ter.5, phrase d'en-tête de livraison), `docs/PRODUCT_SPEC.md` (liste du module V)

**Interfaces:**
- Consumes : `remediationFor`, `fixAllCommands`, `FixAllCommand`, `FindingDetail.remediation` (tâche 1).
- Produces (modèle de vue) : `fixAll: { state: LoadState; commands: FixAllCommand[] }`, `copy: { available: boolean; copy: (text: string) => void; copied: string | null; error: string | null }`.

- [ ] **Step 1: Écrire les tests qui échouent** dans `VulnerabilitiesPanel.test.tsx`, sur le modèle des tests existants (réponses d'API simulées) :
  - un constat `fixed` de type `deb` sélectionné affiche la région « Comment la corriger » avec le texte de la commande `sudo apt update && sudo apt install --only-upgrade openssl` et « Version visée : 3.5.4-1 »
  - un constat `not-fixed` sélectionné affiche « Aucune correction publiée » et aucun élément `code` de commande
  - la carte « Tout corriger » affiche la commande globale quand l'appel `fixable=true&limit=500` renvoie des constats ; elle est absente quand il n'en renvoie aucun
  - « Copier » appelle `navigator.clipboard.writeText` avec la commande et affiche « Commande copiée » (`role="status"`)
  - sans `navigator.clipboard`, aucun bouton « Copier »

- [ ] **Step 2: Lancer les tests et constater l'échec**

Run: `cd frontend && npx vitest run src/features/host/VulnerabilitiesPanel.test.tsx`
Expected: FAIL

- [ ] **Step 3: Implémenter le hook** : requête `[...vulnerabilitiesQueryKey, 'fixable', lastSuccessID]` avec les paramètres des Global Constraints, `fixAllCommands` en `useMemo` ; copie sur le modèle de `frontend/src/features/backups/useBackupSSHKey.ts`.

- [ ] **Step 4: Implémenter l'affichage** : section `aria-label="Comment la corriger"` dans `FindingDetailCard` (résumé, commande en `code` sélectionnable, bouton « Copier », version visée, suite, lien d'avis déjà présent) ; carte « Tout corriger » entre les totaux et les constats, une ligne par `FixAllCommand`, avec la note de repli. Rappeler en une phrase que le Manager n'exécute rien et que la commande se lance sur la machine, en root.

- [ ] **Step 5: Lancer les tests et constater la réussite**

Run: `cd frontend && npx vitest run src/features/host/VulnerabilitiesPanel.test.tsx`
Expected: PASS

- [ ] **Step 6: Documentation** : § 8 ter.5 décrit les deux blocs et rappelle qu'aucune commande n'est exécutée par le Manager (D-1 inchangée) ; PRODUCT_SPEC mentionne « marche à suivre pour corriger ».

- [ ] **Step 7: Vérification**

Run: `scripts/verify.sh` (Node 22)
Expected: code de sortie 0

- [ ] **Step 8: Commit** — `feat(vulnerabilities): show how to fix findings in the Machine page`
