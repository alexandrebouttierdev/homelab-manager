import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { loadState } from '../../hooks/loadState'
import { useSession } from '../../hooks/useSession'
import {
  createChannel,
  deleteChannel,
  listChannels,
  testChannel,
  updateChannel,
  type ChannelInput,
} from '../../lib/api/notifications'
import {
  channelInput,
  channelSecretFields,
  channelToValues,
  channelTypeOptions,
  emptyChannelValues,
  minSeverityOptions,
  toChannelRow,
  toggledChannelInput,
  type ChannelRow,
} from '../../lib/notifications'
import { canManageAlerts } from '../../lib/roles'
import { channelSchema, type ChannelValues } from '../../schemas/notificationChannel'
import { alertsQueryKey } from './queryKeys'

const channelsQueryKey = [...alertsQueryKey, 'channels'] as const

/** Logic and view model of the channels tab of AlertsPage (administrators). */
export function useChannelsPanel() {
  const queryClient = useQueryClient()
  const { user } = useSession()
  const canManage = canManageAlerts(user?.role)
  const channelsQuery = useQuery({
    queryKey: channelsQueryKey,
    queryFn: listChannels,
    enabled: canManage,
  })
  const refresh = () => queryClient.invalidateQueries({ queryKey: channelsQueryKey })

  /** "new", the id of the channel being edited, or null when the form is closed. */
  const [editing, setEditing] = useState<string | null>(null)
  const form = useForm<ChannelValues>({
    resolver: zodResolver(channelSchema),
    defaultValues: emptyChannelValues,
  })
  const type = useWatch({ control: form.control, name: 'type' })
  const definedSecrets = useWatch({ control: form.control, name: 'definedSecrets' })

  const save = useMutation({
    mutationFn: ({ id, input }: { id: string | null; input: ChannelInput }) =>
      id ? updateChannel(id, input) : createChannel(input),
    onSuccess: () => setEditing(null),
    onSettled: refresh,
  })
  const toggle = useMutation({
    mutationFn: ({ id, input }: { id: string; input: ChannelInput }) => updateChannel(id, input),
    onSettled: refresh,
  })
  const [pendingDeletion, setPendingDeletion] = useState<ChannelRow | null>(null)
  const remove = useMutation({
    mutationFn: deleteChannel,
    onSuccess: () => setPendingDeletion(null),
    onSettled: refresh,
  })
  const [testResult, setTestResult] = useState<{ id: string; ok: boolean; message: string } | null>(
    null,
  )
  const test = useMutation({
    mutationFn: testChannel,
    onMutate: () => setTestResult(null),
    onSuccess: (_, id) => setTestResult({ id, ok: true, message: 'Message de test envoyé.' }),
    onError: (error, id) => setTestResult({ id, ok: false, message: error.message }),
    onSettled: refresh,
  })

  const channels = useMemo(() => channelsQuery.data?.channels ?? [], [channelsQuery.data])
  const rows = useMemo(() => channels.map(toChannelRow), [channels])
  const editedChannel = channels.find((channel) => channel.id === editing)

  return {
    canManage,
    state: loadState(channelsQuery),
    rows,
    secretsAvailable: channelsQuery.data?.secretsAvailable ?? true,
    editing,
    isNew: editing === 'new',
    startCreate: () => {
      save.reset()
      form.reset(emptyChannelValues)
      setEditing('new')
    },
    startEdit: (id: string) => {
      const channel = channels.find((candidate) => candidate.id === id)
      if (!channel) return
      save.reset()
      form.reset(channelToValues(channel))
      setEditing(id)
    },
    cancelEdit: () => setEditing(null),
    register: form.register,
    errors: form.formState.errors,
    type,
    typeLabel: channelTypeOptions.find((option) => option.value === type)?.label ?? type,
    secretFields: channelSecretFields(type).map((field) => ({
      ...field,
      defined: definedSecrets.includes(field.name),
      fingerprint: editedChannel?.secrets[field.name]?.fingerprint ?? null,
    })),
    options: { types: channelTypeOptions, severities: minSeverityOptions },
    onSave: form.handleSubmit((values) =>
      save.mutate({ id: editing === 'new' ? null : editing, input: channelInput(values) }),
    ),
    isSaving: save.isPending,
    saveError: save.error?.message ?? null,
    toggleEnabled: (id: string) => {
      const channel = channels.find((candidate) => candidate.id === id)
      if (channel) toggle.mutate({ id, input: toggledChannelInput(channel) })
    },
    isToggling: toggle.isPending,
    actionError: toggle.error?.message ?? remove.error?.message ?? null,
    test: (id: string) => test.mutate(id),
    testingId: test.isPending ? (test.variables ?? null) : null,
    testResult,
    pendingDeletion,
    askDelete: (row: ChannelRow) => {
      remove.reset()
      setPendingDeletion(row)
    },
    cancelDelete: () => setPendingDeletion(null),
    confirmDelete: () => {
      if (pendingDeletion) remove.mutate(pendingDeletion.id)
    },
    isDeleting: remove.isPending,
  }
}

export type ChannelsPanelModel = ReturnType<typeof useChannelsPanel>
