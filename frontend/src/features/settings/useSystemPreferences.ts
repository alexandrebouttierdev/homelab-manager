import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { loadState } from '../../hooks/loadState'
import { useSession } from '../../hooks/useSession'
import { getSystemPreferences, updateSystemPreferences } from '../../lib/api/system'
import { canManageSystem } from '../../lib/roles'
import {
  commonLocales,
  systemChangeConsequence,
  systemChanges,
  systemSettingLabel,
} from '../../lib/systemPreferences'
import {
  systemPreferencesSchema,
  type SystemPreferencesValues,
} from '../../schemas/systemPreferences'

const queryKey = ['system', 'preferences'] as const

/** Host name, time zone and language of the machine (D-6). */
export function useSystemPreferences() {
  const queryClient = useQueryClient()
  const { user } = useSession()
  const query = useQuery({ queryKey, queryFn: getSystemPreferences, retry: false })
  const form = useForm<SystemPreferencesValues>({
    resolver: zodResolver(systemPreferencesSchema),
    values: query.data?.preferences,
  })
  // The values waiting for the confirmation of the administrator.
  const [pending, setPending] = useState<SystemPreferencesValues | null>(null)
  const mutation = useMutation({
    mutationFn: (values: SystemPreferencesValues) => updateSystemPreferences(values),
    onSuccess: (state) => {
      queryClient.setQueryData(queryKey, state)
      setPending(null)
    },
    onError: () => {
      setPending(null)
      void queryClient.invalidateQueries({ queryKey })
    },
  })

  const data = query.data
  const editable = canManageSystem(user?.role) && data?.enabled === true
  const changes = data && pending ? systemChanges(data.preferences, pending) : []
  return {
    state: loadState(query),
    preferences: data?.preferences ?? null,
    timezones: data?.timezones ?? [],
    locales: commonLocales,
    editable,
    /** Debian: the language is set with dpkg-reconfigure locales. */
    localeEditable: editable && data?.localeEditable !== false,
    localeLocked: data?.localeEditable === false,
    /** Shown to administrators while the setting is off. */
    disabledNotice: canManageSystem(user?.role) && data?.enabled === false,
    register: form.register,
    errors: form.formState.errors,
    isDirty: form.formState.isDirty,
    onSubmit: form.handleSubmit((values) => {
      mutation.reset()
      if (data && systemChanges(data.preferences, values).length > 0) setPending(values)
    }),
    confirmation:
      changes.length > 0
        ? changes.map((change) => ({
            label: systemSettingLabel(change.setting),
            from: change.from,
            to: change.to,
            consequence: systemChangeConsequence(change.setting),
          }))
        : null,
    confirm: () => {
      if (pending) mutation.mutate(pending)
    },
    cancel: () => setPending(null),
    isSaving: mutation.isPending,
    errorMessage: mutation.error?.message ?? null,
    saved:
      mutation.isSuccess && mutation.data.changed
        ? mutation.data.changed.map(systemSettingLabel).join(', ') || 'Aucun changement'
        : null,
  }
}

export type SystemPreferencesModel = ReturnType<typeof useSystemPreferences>
