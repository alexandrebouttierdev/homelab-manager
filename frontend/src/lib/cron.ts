import type { BadgeTone } from '../components/ui/Badge'
import type { CronDirectory, CronEntry, CronTimer } from './api/cron'

const periodLabels: Record<CronDirectory['period'], string> = {
  hourly: 'Scripts horaires',
  daily: 'Scripts quotidiens',
  weekly: 'Scripts hebdomadaires',
  monthly: 'Scripts mensuels',
}

export function periodLabel(period: CronDirectory['period']): string {
  return periodLabels[period]
}

/** How a run-parts directory is started: its crontab line, anacron, or nothing found. */
export function directorySchedule(directory: CronDirectory, anacron: boolean): string {
  const line = directory.scheduledBy
  const scheduled = line?.description ?? null
  if (anacron && directory.period !== 'hourly')
    return scheduled
      ? `Par anacron (à défaut : ${scheduled.toLowerCase()})`
      : 'Par anacron, qui rattrape un passage manqué'
  return scheduled ?? 'Aucune ligne de crontab ne lance ce répertoire'
}

/** Entries matching a search on the command, the account, the file or the schedule. */
export function filterCronEntries(entries: CronEntry[], search: string): CronEntry[] {
  const needle = search.trim().toLowerCase()
  if (!needle) return entries
  return entries.filter((entry) =>
    [entry.command, entry.user, entry.source, entry.expression, entry.description ?? ''].some(
      (text) => text.toLowerCase().includes(needle),
    ),
  )
}

const resultLabels: Record<string, { label: string; tone: BadgeTone }> = {
  success: { label: 'Réussi', tone: 'success' },
  'exit-code': { label: 'Code de sortie non nul', tone: 'danger' },
  signal: { label: 'Interrompu par un signal', tone: 'danger' },
  'core-dump': { label: 'Plantage', tone: 'danger' },
  timeout: { label: 'Délai dépassé', tone: 'danger' },
  'start-limit-hit': { label: 'Trop de démarrages', tone: 'danger' },
}

/** Result of the last run of the service a timer starts. */
export function timerResult(timer: CronTimer): { label: string; tone: BadgeTone } {
  if (!timer.serviceResult) return { label: 'Inconnu', tone: 'neutral' }
  return resultLabels[timer.serviceResult] ?? { label: timer.serviceResult, tone: 'warning' }
}

/** Timers sorted by next run, those without one last. */
export function sortTimers(timers: CronTimer[]): CronTimer[] {
  return [...timers].sort((a, b) => {
    if (a.nextElapse === b.nextElapse) return a.name.localeCompare(b.name)
    if (!a.nextElapse) return 1
    if (!b.nextElapse) return -1
    return a.nextElapse.localeCompare(b.nextElapse)
  })
}
