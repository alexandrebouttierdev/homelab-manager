import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { loadState } from '../../hooks/loadState'
import { useSession } from '../../hooks/useSession'
import { getVulnerabilitySummary, updateScanInterval } from '../../lib/api/vulnerabilities'
import { formatScanInterval } from '../../lib/vulnerabilities'
import { scanIntervalSchema, type ScanIntervalValues } from '../../schemas/scanInterval'
import { vulnerabilitiesQueryKey } from '../host/useVulnerabilitiesPanel'

/** Interval of the automatic vulnerability scans: reading for everyone, changing for administrators. */
export function useScanIntervalForm() {
  const queryClient = useQueryClient()
  const { canManageSettings } = useSession()
  const summary = useQuery({
    queryKey: [...vulnerabilitiesQueryKey, 'summary'],
    queryFn: getVulnerabilitySummary,
  })
  const form = useForm<ScanIntervalValues>({
    resolver: zodResolver(scanIntervalSchema),
    values: summary.data ? { intervalHours: summary.data.intervalHours } : undefined,
  })
  const mutation = useMutation({
    mutationFn: (values: ScanIntervalValues) => updateScanInterval(values.intervalHours),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: vulnerabilitiesQueryKey }),
  })
  return {
    state: loadState(summary),
    current: summary.data ? formatScanInterval(summary.data.intervalHours) : null,
    defaultInterval: summary.data ? formatScanInterval(summary.data.defaultIntervalHours) : null,
    customised: summary.data?.intervalSource === 'user',
    register: form.register,
    errors: form.formState.errors,
    isDirty: form.formState.isDirty,
    onSubmit: form.handleSubmit((values) => mutation.mutate(values)),
    isPending: mutation.isPending,
    errorMessage: mutation.error?.message ?? null,
    saved: mutation.isSuccess && !form.formState.isDirty,
    readOnly: !canManageSettings,
  }
}
