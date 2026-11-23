import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { loadState } from '../../hooks/loadState'
import { useSession } from '../../hooks/useSession'
import { getPowerSettings, requestPowerAction, type PowerAction } from '../../lib/api/machine'
import { powerActionList, powerActionTexts } from '../../lib/power'
import { powerConfirmationSchema, type PowerConfirmationValues } from '../../schemas/power'

/** Power card of the Machine page (administrators only). */
export function usePowerActions(hostname: string) {
  const { canManagePower } = useSession()
  const settings = useQuery({
    queryKey: ['machine', 'power'],
    queryFn: getPowerSettings,
    enabled: canManagePower,
    staleTime: 60_000,
  })
  const [selected, setSelected] = useState<PowerAction | null>(null)
  const schema = useMemo(() => powerConfirmationSchema(hostname), [hostname])
  const form = useForm<PowerConfirmationValues>({
    resolver: zodResolver(schema),
    defaultValues: { confirmHostname: '' },
  })
  const mutation = useMutation({
    mutationFn: (request: { action: PowerAction; confirmHostname: string }) =>
      requestPowerAction(request.action, request.confirmHostname),
  })

  return {
    visible: canManagePower,
    state: loadState(settings),
    enabled: settings.data?.enabled ?? false,
    hostname,
    actions: powerActionList.map((action) => ({ action, label: powerActionTexts[action].label })),
    selected: selected ? { action: selected, ...powerActionTexts[selected] } : null,
    choose: (action: PowerAction) => {
      mutation.reset()
      form.reset()
      setSelected(action)
    },
    cancel: () => {
      form.reset()
      setSelected(null)
    },
    register: form.register,
    errors: form.formState.errors,
    onConfirm: form.handleSubmit((values) => {
      if (selected) mutation.mutate({ action: selected, confirmHostname: values.confirmHostname })
    }),
    isPending: mutation.isPending,
    /** Once a request is accepted the Manager goes down: no second request. */
    accepted: mutation.isSuccess,
    errorMessage: mutation.error?.message ?? null,
    doneMessage: mutation.data ? powerActionTexts[mutation.data.action].done : null,
  }
}
