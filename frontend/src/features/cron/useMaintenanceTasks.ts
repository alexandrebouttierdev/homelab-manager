import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { loadState } from '../../hooks/loadState'
import { useSession } from '../../hooks/useSession'
import type { BackupSchedule } from '../../lib/api/backups'
import {
  listMaintenanceTasks,
  runMaintenanceTask,
  updateMaintenanceTask,
  type MaintenanceTask,
} from '../../lib/api/maintenance'
import { describeSchedule, weekdayOptions } from '../../lib/backups'
import {
  lastRunInfo,
  maintenanceDescription,
  maintenanceTitle,
  sizeLabel,
} from '../../lib/maintenance'

const queryKey = ['maintenance', 'tasks'] as const
const timePattern = /^([01]\d|2[0-3]):[0-5]\d$/

type Draft = { schedule: BackupSchedule; maxSizeMiB: number }

/** The maintenance catalog (decision D-1): schedules set by administrators. */
export function useMaintenanceTasks() {
  const queryClient = useQueryClient()
  const { canManageSettings } = useSession()
  const query = useQuery({
    queryKey,
    queryFn: listMaintenanceTasks,
    refetchInterval: (state) => (state.state.data?.some((task) => task.running) ? 2000 : false),
  })
  const [drafts, setDrafts] = useState<Partial<Record<MaintenanceTask['kind'], Draft>>>({})
  const save = useMutation({
    mutationFn: ({ kind, draft }: { kind: MaintenanceTask['kind']; draft: Draft }) =>
      updateMaintenanceTask(kind, draft),
    onSuccess: (task) => {
      setDrafts((current) => ({ ...current, [task.kind]: undefined }))
      void queryClient.invalidateQueries({ queryKey })
    },
  })
  const run = useMutation({
    mutationFn: (kind: MaintenanceTask['kind']) => runMaintenanceTask(kind),
    onSettled: () => queryClient.invalidateQueries({ queryKey }),
  })

  const tasks = (query.data ?? []).map((task) => {
    const draft = drafts[task.kind] ?? { schedule: task.schedule, maxSizeMiB: task.maxSizeMiB }
    const change = (next: { schedule?: Partial<BackupSchedule>; maxSizeMiB?: number }) =>
      setDrafts((current) => ({
        ...current,
        [task.kind]: {
          schedule: { ...draft.schedule, ...next.schedule },
          maxSizeMiB: next.maxSizeMiB ?? draft.maxSizeMiB,
        },
      }))
    return {
      kind: task.kind,
      title: maintenanceTitle(task),
      description: maintenanceDescription(task),
      unit: task.unit,
      schedule: describeSchedule(task.schedule),
      nextRunAt: task.nextRunAt,
      lastRun: lastRunInfo(task),
      lastRunAt: task.lastRun?.at ?? null,
      sizeOptions: task.sizes.map((size) => ({ value: size, label: sizeLabel(size) })),
      currentSize: sizeLabel(task.maxSizeMiB),
      draft,
      dirty: drafts[task.kind] !== undefined,
      timeError: timePattern.test(draft.schedule.time) ? null : 'Heure au format HH:MM',
      setEnabled: (enabled: boolean) => change({ schedule: { enabled } }),
      setFrequency: (frequency: BackupSchedule['frequency']) => change({ schedule: { frequency } }),
      setWeekday: (weekday: number) => change({ schedule: { weekday } }),
      setTime: (time: string) => change({ schedule: { time } }),
      setSize: (maxSizeMiB: number) => change({ maxSizeMiB }),
      save: () => save.mutate({ kind: task.kind, draft }),
      run: () => run.mutate(task.kind),
      running: task.running || (run.isPending && run.variables === task.kind),
    }
  })

  return {
    state: loadState(query),
    tasks,
    editable: canManageSettings,
    weekdayOptions,
    saving: save.isPending,
    error: save.error?.message ?? run.error?.message ?? null,
  }
}

export type MaintenanceModel = ReturnType<typeof useMaintenanceTasks>
