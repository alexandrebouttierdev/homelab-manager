import { requestJSON } from './client'

/** A line of a system crontab (API_CONTRACTS.md § 4.11); the command has its secrets masked. */
export type CronEntry = {
  source: string
  line: number
  expression: string
  user: string
  command: string
  valid: boolean
  error: string | null
  description: string | null
  nextRuns: string[]
  /** From the CRON lines of the journal; null when none was found. */
  lastRun: string | null
}

export type CronDirectory = {
  period: 'hourly' | 'daily' | 'weekly' | 'monthly'
  path: string
  scripts: string[]
  scheduledBy: CronEntry | null
}

export type CronTimer = {
  name: string
  description: string
  activeState: string
  nextElapse: string | null
  lastTrigger: string | null
  service: string | null
  serviceResult: string | null
}

export type CronInventory = {
  timeZone: string
  entries: CronEntry[]
  directories: CronDirectory[]
  anacron: boolean
  unreadable: string[]
  timers: CronTimer[]
  timersAvailable: boolean
  journalAvailable: boolean
}

export type CronInspection = { description: string; nextRuns: string[] }

export function getCron() {
  return requestJSON<CronInventory>('/api/v1/cron')
}

export function inspectCron(expression: string) {
  return requestJSON<CronInspection>(
    `/api/v1/cron/inspect?${new URLSearchParams({ expression }).toString()}`,
  )
}
