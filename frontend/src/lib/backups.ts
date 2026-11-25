import type { BadgeTone } from '../components/ui/Badge'
import type {
  BackupConfig,
  BackupConfigInput,
  BackupCopyConfig,
  BackupCopyInput,
  BackupRepository,
  BackupDiff,
  BackupDiffModifier,
  BackupFailure,
  BackupOrigin,
  BackupRun,
  BackupSchedule,
  BackupStats,
  BackupSummary,
  RepositoryType,
} from './api/backups'
import { formatBytes, formatCount, formatRelativeTime } from './format'
import { splitLines, type BackupConfigValues } from '../schemas/backupConfig'
import type { BackupCopyValues } from '../schemas/backupCopy'

export const repositoryTypeOptions: ReadonlyArray<{
  value: BackupConfigValues['type']
  label: string
}> = [
  { value: 'none', label: 'Aucun dépôt' },
  { value: 'local', label: 'Répertoire local ou monté' },
  { value: 'rest', label: 'rest-server (HTTPS)' },
  { value: 's3', label: 'S3 compatible (dont Backblaze B2)' },
  { value: 'sftp', label: 'SFTP (serveur SSH)' },
]

const repositoryLabels: Record<RepositoryType, string> = {
  local: 'Répertoire local',
  rest: 'rest-server',
  s3: 'S3',
  sftp: 'SFTP',
}

export function repositoryLabel(type: RepositoryType): string {
  return repositoryLabels[type]
}

/** Names of the access keys of a repository type; a local repository has none. */
export function credentialLabels(
  type: BackupConfigValues['type'],
): { username: string; secret: string } | null {
  if (type === 'rest') return { username: 'Utilisateur (facultatif)', secret: 'Mot de passe' }
  if (type === 's3') return { username: "Clé d'accès (access key ID)", secret: 'Clé secrète' }
  return null
}

const locationHints: Record<BackupConfigValues['type'], string> = {
  none: '',
  local: 'Chemin absolu, par exemple /mnt/sauvegarde/restic',
  rest: "Adresse https://hôte:port/chemin d'un rest-server",
  s3: 'Adresse https://hôte/bucket[/préfixe]',
  sftp: 'sftp://utilisateur@hôte[:port]/chemin/absolu ; le Manager se connecte avec sa clé SSH',
}

/** What the location field of a repository type expects. */
export function locationHint(type: BackupConfigValues['type']): string {
  return locationHints[type]
}

export const weekdayOptions: ReadonlyArray<{ value: number; label: string }> = [
  { value: 1, label: 'Lundi' },
  { value: 2, label: 'Mardi' },
  { value: 3, label: 'Mercredi' },
  { value: 4, label: 'Jeudi' },
  { value: 5, label: 'Vendredi' },
  { value: 6, label: 'Samedi' },
  { value: 0, label: 'Dimanche' },
]

export function describeSchedule(schedule: BackupSchedule): string {
  if (!schedule.enabled) return 'Désactivée'
  if (schedule.frequency === 'daily') return `Chaque jour à ${schedule.time}`
  const day = weekdayOptions.find((option) => option.value === schedule.weekday)?.label ?? ''
  return `Chaque ${day.toLowerCase()} à ${schedule.time}`
}

export function runStateInfo(run: BackupRun): { label: string; tone: BadgeTone } {
  if (run.state === 'running') return { label: 'En cours', tone: 'info' }
  if (run.state === 'failed') return { label: 'Échec', tone: 'danger' }
  if (run.warning) return { label: 'Réussie, fichiers ignorés', tone: 'warning' }
  return { label: 'Réussie', tone: 'success' }
}

const failureLabels: Record<BackupFailure, string> = {
  runner_not_installed: 'homelab-backup-runner ou son unité est absent (voir le README)',
  not_allowed: "polkit a refusé : la règle 50-homelab-manager-backup.rules n'est pas installée",
  restic_failed: 'restic a échoué',
  timeout: "l'opération a dépassé sa durée maximale",
  invalid_result: "le résultat de l'opération est absent ou illisible",
  secrets_unavailable: 'la clé du coffre est absente : le mot de passe du dépôt est illisible',
  job_failed: "le fichier de travail n'a pas pu être écrit",
  systemd_unavailable: "systemd n'est pas joignable sur le bus système",
  interrupted: "le Manager a été arrêté pendant l'opération",
}

/** Why a run failed, with restic's own message when it gave one. */
export function runFailureText(run: BackupRun): string | null {
  if (run.state !== 'failed') return run.warning ? run.message : null
  const reason = run.failure ? failureLabels[run.failure] : 'cause inconnue'
  return run.message ? `${reason} : ${run.message}` : reason
}

const originLabels: Record<BackupOrigin, string> = {
  manual: 'Manuelle',
  schedule: 'Planifiée',
  catchup: 'Rattrapage',
}

const operationLabels: Record<Exclude<BackupRun['operation'], 'backup'>, string> = {
  init: 'Initialisation du dépôt',
  'forget-prune': 'Rétention',
  check: 'Vérification du dépôt',
  restore: 'Restauration',
  snapshots: 'Rattachement du dépôt',
  unlock: 'Déverrouillage',
  diff: "Comparaison d'instantanés",
  copy: 'Copie vers le dépôt de copie',
  'restore-in-place': "Restauration à l'emplacement d'origine",
  dump: 'Archive à télécharger',
}

export function runLabel(run: BackupRun): string {
  if (run.operation === 'backup') return `Sauvegarde ${originLabels[run.origin].toLowerCase()}`
  if (run.operation === 'restore' && run.details?.dryRun) return 'Restauration simulée'
  if (run.operation === 'restore-in-place' && run.details?.dryRun)
    return "Restauration à l'emplacement d'origine simulée"
  const label = operationLabels[run.operation]
  const scheduled =
    run.origin !== 'manual' &&
    (run.operation === 'forget-prune' || run.operation === 'check' || run.operation === 'copy')
  return scheduled ? `${label} planifiée` : label
}

/** What an operation other than a backup reports. */
export function describeDetails(run: BackupRun): string | null {
  const d = run.details
  if (!d || run.state !== 'succeeded') return null
  switch (run.operation) {
    case 'restore':
      return d.dryRun
        ? `Simulation : ${formatCount(d.totalFiles ?? 0)} fichier(s), ${formatBytes(d.totalBytes ?? 0)}`
        : `${formatCount(d.filesRestored ?? 0)} fichier(s) restauré(s) dans ${d.target ?? '—'}`
    case 'restore-in-place':
      // restic counts the directories in files_restored: the files of
      // the selection are total_files.
      if (d.dryRun)
        return `Simulation : ${formatCount(d.totalFiles ?? 0)} fichier(s) concerné(s), ${formatBytes(d.totalBytes ?? 0)}`
      return [
        `${formatCount(d.totalFiles ?? 0)} fichier(s) remis à leur emplacement d'origine`,
        d.safetySnapshotId
          ? `état précédent sauvegardé dans l'instantané ${d.safetySnapshotId.slice(0, 8)}`
          : null,
      ]
        .filter(Boolean)
        .join(' · ')
    case 'dump':
      return `Archive de ${d.archivePath ?? '—'} (${formatBytes(d.archiveBytes ?? 0)})`
    case 'forget-prune':
      return `${formatCount(d.snapshotsRemoved ?? 0)} instantané(s) supprimé(s)`
    case 'check':
      return `${d.subsetPercent ?? 0} % des données relues sans erreur`
    case 'diff':
      return d.diff ? describeDiff(d.diff) : null
    case 'copy':
      if (!d.copy) return null
      return [
        d.copy.initialized ? 'Dépôt de copie créé' : null,
        `${formatCount(d.copy.snapshotsCopied)} instantané(s) copié(s)`,
        d.copy.snapshotsRemoved
          ? `${formatCount(d.copy.snapshotsRemoved)} supprimé(s) par la rétention`
          : null,
        `${formatCount(d.copy.snapshotsInTarget)} dans la copie`,
      ]
        .filter(Boolean)
        .join(' · ')
    default:
      return null
  }
}

/** One line for a diff: changed, added and removed files. */
export function describeDiff(diff: BackupDiff): string {
  return `${formatCount(diff.changedFiles)} modifié(s), ${formatCount(diff.added.files)} ajouté(s) (${formatBytes(diff.added.bytes)}), ${formatCount(diff.removed.files)} supprimé(s) (${formatBytes(diff.removed.bytes)})`
}

const modifierInfo: Record<BackupDiffModifier, { label: string; tone: BadgeTone }> = {
  '+': { label: 'Ajouté', tone: 'success' },
  '-': { label: 'Supprimé', tone: 'danger' },
  M: { label: 'Modifié', tone: 'warning' },
  U: { label: 'Métadonnées', tone: 'neutral' },
  T: { label: 'Type changé', tone: 'info' },
  '?': { label: 'Contenu altéré', tone: 'danger' },
}

/** What a modifier of restic diff means. */
export function diffModifierInfo(modifier: BackupDiffModifier): { label: string; tone: BadgeTone } {
  return modifierInfo[modifier] ?? { label: modifier, tone: 'neutral' }
}

/** The two snapshots of a comparison, older first, whatever the order they were picked in. */
export function orderForDiff<T extends { id: string; time: string }>(a: T, b: T): [T, T] {
  return new Date(a.time).getTime() <= new Date(b.time).getTime() ? [a, b] : [b, a]
}

/** The repository of a form; only an SFTP repository carries a host key. */
function repositoryOf(type: RepositoryType, location: string, hostKey: string): BackupRepository {
  const repository: BackupRepository = { type, location: location.trim() }
  if (type === 'sftp') repository.hostKey = hostKey.trim()
  return repository
}

export function copyToValues(copy: BackupCopyConfig | null): BackupCopyValues {
  return {
    type: copy?.repository.type ?? 'local',
    location: copy?.repository.location ?? '',
    hostKey: copy?.repository.hostKey ?? '',
    username: '',
    secret: '',
    afterBackup: copy?.afterBackup ?? true,
  }
}

/** An empty key field keeps the stored key; a local repository removes the keys. */
export function copyValuesToInput(values: BackupCopyValues): BackupCopyInput {
  const remote = values.type === 'rest' || values.type === 's3'
  return {
    repository: repositoryOf(values.type, values.location, values.hostKey),
    username: remote ? values.username || null : '',
    secret: remote ? values.secret || null : '',
    afterBackup: values.afterBackup,
  }
}

export function formatDuration(seconds: number | null | undefined): string {
  if (seconds === null || seconds === undefined) return '—'
  if (seconds < 60) return `${Math.max(1, Math.round(seconds))} s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes} min ${Math.round(seconds % 60)} s`
  return `${Math.floor(minutes / 60)} h ${minutes % 60} min`
}

export function runDurationSeconds(run: BackupRun): number | null {
  if (!run.finishedAt) return null
  return (new Date(run.finishedAt).getTime() - new Date(run.startedAt).getTime()) / 1000
}

export function describeStats(stats: BackupStats | null): string {
  if (!stats) return '—'
  return `${formatCount(stats.filesNew)} nouveau(x), ${formatCount(stats.filesChanged)} modifié(s) · ${formatBytes(stats.dataAdded)} ajoutés`
}

export type BackupHealth = { label: string; tone: BadgeTone; detail: string }

/** The state shown at the top of the page and on the dashboard. */
export function backupHealth(summary: BackupSummary): BackupHealth {
  if (!summary.configured)
    return { label: 'Non configurées', tone: 'neutral', detail: 'Aucun dépôt défini.' }
  if (!summary.initialized)
    return {
      label: 'Dépôt à initialiser',
      tone: 'warning',
      detail: "Le dépôt n'est pas encore initialisé.",
    }
  if (summary.running)
    return { label: 'En cours', tone: 'info', detail: 'Une opération est en cours.' }
  if (summary.lastRun?.state === 'failed')
    return {
      label: 'Dernière en échec',
      tone: 'danger',
      detail: summary.lastSuccess
        ? `Dernier succès ${formatRelativeTime(summary.lastSuccess.finishedAt)}.`
        : 'Aucune sauvegarde réussie.',
    }
  if (!summary.lastSuccess)
    return { label: 'Aucune sauvegarde', tone: 'warning', detail: 'Aucune sauvegarde réussie.' }
  return {
    label: 'À jour',
    tone: 'success',
    detail: `Dernier succès ${formatRelativeTime(summary.lastSuccess.finishedAt)}.`,
  }
}

export function configToValues(config: BackupConfig): BackupConfigValues {
  return {
    type: config.repository?.type ?? 'none',
    location: config.repository?.location ?? '',
    hostKey: config.repository?.hostKey ?? '',
    username: '',
    secret: '',
    sources: config.sources.join('\n'),
    excludes: config.excludes.join('\n'),
    excludeCaches: config.excludeCaches,
    excludeIfPresent: config.excludeIfPresent,
    sqliteDatabases: (config.sqliteDatabases ?? []).join('\n'),
    scheduleEnabled: config.schedule.enabled,
    frequency: config.schedule.frequency,
    weekday: config.schedule.weekday,
    time: config.schedule.time,
    retentionEnabled: config.retention.enabled,
    keepDaily: config.retention.keepDaily,
    keepWeekly: config.retention.keepWeekly,
    keepMonthly: config.retention.keepMonthly,
    keepYearly: config.retention.keepYearly,
    checkEnabled: config.check.enabled,
    checkSubsetPercent: config.check.subsetPercent,
  }
}

/**
 * The request of a form: an empty key field keeps the stored key; a local
 * repository (or none) removes the keys.
 */
export function valuesToInput(values: BackupConfigValues): BackupConfigInput {
  const remote = values.type === 'rest' || values.type === 's3'
  return {
    repository:
      values.type === 'none' ? null : repositoryOf(values.type, values.location, values.hostKey),
    username: remote ? values.username || null : '',
    secret: remote ? values.secret || null : '',
    sources: splitLines(values.sources),
    excludes: splitLines(values.excludes),
    excludeCaches: values.excludeCaches,
    excludeIfPresent: values.excludeIfPresent.trim(),
    sqliteDatabases: splitLines(values.sqliteDatabases),
    schedule: {
      enabled: values.scheduleEnabled,
      frequency: values.frequency,
      weekday: values.weekday,
      time: values.time,
    },
    retention: {
      enabled: values.retentionEnabled,
      keepDaily: values.keepDaily,
      keepWeekly: values.keepWeekly,
      keepMonthly: values.keepMonthly,
      keepYearly: values.keepYearly,
    },
    check: { enabled: values.checkEnabled, subsetPercent: values.checkSubsetPercent },
  }
}

/** How long an archive stays downloadable (API_CONTRACTS.md § 4.10). */
export const archiveRetentionMs = 24 * 60 * 60 * 1000

/** The download address of a dump run, while its archive is kept. */
export function archiveUrl(run: BackupRun, now: Date): string | null {
  if (run.operation !== 'dump' || run.state !== 'succeeded' || !run.finishedAt) return null
  if (now.getTime() - new Date(run.finishedAt).getTime() > archiveRetentionMs) return null
  return `/api/v1/backups/runs/${encodeURIComponent(run.id)}/archive`
}

/**
 * Trees an in-place restore never writes to (mirror of the server, which
 * decides): the system, the virtual file systems and the state of the
 * Manager and of its backups.
 */
const inPlaceProtected = [
  '/bin',
  '/boot',
  '/dev',
  '/efi',
  '/etc',
  '/lib',
  '/lib32',
  '/lib64',
  '/libx32',
  '/proc',
  '/run',
  '/sbin',
  '/sys',
  '/tmp',
  '/usr',
  '/var/cache/homelab-backup',
  '/var/lib/homelab-backup',
  '/var/lib/homelab-manager',
  '/var/lib/homelab-vulnscan',
]

/** Why paths cannot be restored in place, or null. */
export function inPlaceIssue(paths: string[]): string | null {
  if (paths.length === 0)
    return "Nommez les chemins à remettre en place : jamais tout l'instantané."
  for (const path of paths) {
    if (path === '/') return 'La machine entière ne se restaure pas en place.'
    const touched = inPlaceProtected.find(
      (protectedPath) =>
        path === protectedPath ||
        path.startsWith(`${protectedPath}/`) ||
        protectedPath.startsWith(`${path}/`),
    )
    if (touched)
      return `« ${path} » touche ${touched}, jamais restauré en place : passez par la quarantaine.`
  }
  return null
}
