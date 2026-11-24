import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { createSilence } from '../../lib/api/notifications'
import {
  emptySilenceValues,
  silenceDurationOptions,
  silenceInput,
  silenceScopeOptions,
  type SilenceSubject,
} from '../../lib/notifications'
import { silenceSchema, type SilenceValues } from '../../schemas/silence'
import { alertsQueryKey } from './queryKeys'

/** Form that poses a silence on one alert (its target or its rule), or on every alert. */
export function useSilenceForm() {
  const queryClient = useQueryClient()
  /** undefined: closed; null: global silence; else the alert silenced. */
  const [subject, setSubject] = useState<SilenceSubject | undefined>(undefined)
  const form = useForm<SilenceValues>({
    resolver: zodResolver(silenceSchema),
    defaultValues: emptySilenceValues(null),
  })
  const create = useMutation({
    mutationFn: createSilence,
    onSuccess: () => setSubject(undefined),
    onSettled: () => queryClient.invalidateQueries({ queryKey: alertsQueryKey }),
  })
  const current = subject ?? null

  return {
    isOpen: subject !== undefined,
    title: current ? `Silence sur « ${current.ruleName} »` : 'Silence global',
    subject: current ? current.target : 'Toutes les alertes de toutes les règles',
    scopes: silenceScopeOptions(current),
    durations: silenceDurationOptions,
    open: (next: SilenceSubject) => {
      create.reset()
      form.reset(emptySilenceValues(next))
      setSubject(next)
    },
    close: () => setSubject(undefined),
    register: form.register,
    errors: form.formState.errors,
    onSubmit: form.handleSubmit((values) => create.mutate(silenceInput(values, current))),
    isSaving: create.isPending,
    error: create.error?.message ?? null,
  }
}

export type SilenceFormModel = ReturnType<typeof useSilenceForm>
