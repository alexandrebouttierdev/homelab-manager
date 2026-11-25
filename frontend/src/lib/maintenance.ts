import type { BadgeTone } from '../components/ui/Badge'
import type { MaintenanceTask } from './api/maintenance'

const taskTexts: Record<MaintenanceTask['kind'], { title: string; description: string }> = {
  'journal-vacuum': {
    title: 'Nettoyage du journal système',
    description:
      'Supprime les fichiers archivés les plus anciens du journal systemd pour le garder sous la taille choisie (journalctl --vacuum-size).',
  },
}

export function maintenanceTitle(task: MaintenanceTask): string {
  return taskTexts[task.kind].title
}

export function maintenanceDescription(task: MaintenanceTask): string {
  return taskTexts[task.kind].description
}

/** A size in MiB as shown in the list. */
export function sizeLabel(mib: number): string {
  return mib >= 1024 ? `${mib / 1024} Gio` : `${mib} Mio`
}

/** The last run, as shown under the task. */
export function lastRunInfo(task: MaintenanceTask): { label: string; tone: BadgeTone } {
  if (task.running) return { label: 'En cours', tone: 'info' }
  const run = task.lastRun
  if (!run) return { label: 'Jamais lancée', tone: 'neutral' }
  const origin = run.origin === 'manual' ? 'manuelle' : 'planifiée'
  if (run.succeeded) return { label: `Réussie (${origin})`, tone: 'success' }
  return { label: `Échec (${origin}) : ${run.message ?? 'cause inconnue'}`, tone: 'danger' }
}
