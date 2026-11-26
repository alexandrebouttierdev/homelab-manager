import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useForm, useWatch } from 'react-hook-form'
import { useSession } from '../../hooks/useSession'
import { updateRetention, type MonitoringSettings } from '../../lib/api/monitoring'
import {
  estimatedStorageMegabytes,
  retentionSchema,
  type RetentionValues,
} from '../../schemas/retention'
import { settingsQueryKey } from './useSettings'

/** Retention form: validation, storage estimate and saving. */
export function useRetentionForm(settings: MonitoringSettings) {
  const queryClient = useQueryClient()
  const { canManageSettings } = useSession()
  const form = useForm<RetentionValues>({
    resolver: zodResolver(retentionSchema),
    values: { metricsRetentionDays: settings.metricsRetentionDays },
  })
  const mutation = useMutation({
    mutationFn: (values: RetentionValues) => updateRetention(values.metricsRetentionDays),
    onSuccess: (updated) => queryClient.setQueryData(settingsQueryKey, updated),
  })

  const days = useWatch({ control: form.control, name: 'metricsRetentionDays' })
  const invalid = Boolean(form.formState.errors.metricsRetentionDays)
  return {
    register: form.register,
    errors: form.formState.errors,
    isDirty: form.formState.isDirty,
    onSubmit: form.handleSubmit((values) => mutation.mutate(values)),
    isPending: mutation.isPending,
    errorMessage: mutation.error?.message ?? null,
    saved: mutation.isSuccess && !form.formState.isDirty,
    /** Estimated database size in MB, or null while the input is invalid. */
    estimate:
      Number.isFinite(days) && !invalid
        ? estimatedStorageMegabytes(days, settings.systemCollectIntervalSeconds)
        : null,
    customised: settings.metricsRetentionSource === 'user',
    /** Only administrators may change the retention; the server enforces it. */
    readOnly: !canManageSettings,
  }
}
