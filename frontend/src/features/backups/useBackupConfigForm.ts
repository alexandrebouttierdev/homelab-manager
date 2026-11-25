import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import {
  connectBackupRepository,
  initBackupRepository,
  requestBackupOperation,
  updateBackupConfig,
  type AdminBackupOperation,
  type BackupConfig,
} from '../../lib/api/backups'
import {
  configToValues,
  credentialLabels,
  repositoryTypeOptions,
  valuesToInput,
  weekdayOptions,
} from '../../lib/backups'
import { backupConfigSchema, type BackupConfigValues } from '../../schemas/backupConfig'

const backupsQueryKey = ['backups'] as const

/** Configuration tab: administrators edit, the others read. */
export function useBackupConfigForm(config: BackupConfig | undefined, canManage: boolean) {
  const queryClient = useQueryClient()
  // The password generated at initialisation, shown this once.
  const [password, setPassword] = useState<string | null>(null)
  // Password of an existing repository to connect, typed by the administrator.
  const [existingPassword, setExistingPassword] = useState('')
  const [confirmUnlock, setConfirmUnlock] = useState(false)
  const form = useForm<BackupConfigValues>({
    resolver: zodResolver(backupConfigSchema),
    values: config ? configToValues(config) : undefined,
    resetOptions: { keepDirtyValues: true },
  })
  const save = useMutation({
    mutationFn: updateBackupConfig,
    onSuccess: (saved) => {
      queryClient.setQueryData([...backupsQueryKey, 'config'], saved)
      form.reset(configToValues(saved))
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: [...backupsQueryKey, 'summary'] }),
  })
  const init = useMutation({
    mutationFn: initBackupRepository,
    onSuccess: (result) => setPassword(result.password),
    onSettled: () => queryClient.invalidateQueries({ queryKey: backupsQueryKey }),
  })
  const connect = useMutation({
    mutationFn: connectBackupRepository,
    onSuccess: () => setExistingPassword(''),
    onSettled: () => queryClient.invalidateQueries({ queryKey: backupsQueryKey }),
  })
  const operation = useMutation({
    mutationFn: (op: Exclude<AdminBackupOperation, 'restore'>) => requestBackupOperation(op),
    onSuccess: () => setConfirmUnlock(false),
    onSettled: () => queryClient.invalidateQueries({ queryKey: backupsQueryKey }),
  })
  const type = useWatch({ control: form.control, name: 'type' })
  const retentionEnabled = useWatch({ control: form.control, name: 'retentionEnabled' })
  const checkEnabled = useWatch({ control: form.control, name: 'checkEnabled' })
  const frequency = useWatch({ control: form.control, name: 'frequency' })
  const scheduleEnabled = useWatch({ control: form.control, name: 'scheduleEnabled' })

  return {
    loaded: config !== undefined,
    canManage,
    register: form.register,
    errors: form.formState.errors,
    typeOptions: repositoryTypeOptions,
    weekdayOptions,
    credentials: credentialLabels(type),
    /** Placeholders tell a stored key apart from a missing one, never its value. */
    usernameSet: config?.credentials.username ?? false,
    secretSet: config?.credentials.secret ?? false,
    showWeekday: frequency === 'weekly',
    scheduleEnabled,
    onSubmit: form.handleSubmit((values) => save.mutate(valuesToInput(values))),
    isSaving: save.isPending,
    saved: save.isSuccess && !form.formState.isDirty,
    saveError: save.error?.message ?? null,
    secretsAvailable: config?.secretsAvailable ?? true,
    /** Initialisation is offered once a repository is saved and not yet initialised. */
    canInit:
      canManage && Boolean(config?.repository) && !config?.initialized && !form.formState.isDirty,
    initialized: config?.initialized ?? false,
    init: () => init.mutate(),
    isInitializing: init.isPending,
    initError: init.error?.message ?? null,
    password,
    dismissPassword: () => setPassword(null),
    retentionEnabled,
    checkEnabled,
    /** Connect an existing repository instead of initialising a new one. */
    existingPassword,
    setExistingPassword,
    connect: () => connect.mutate(existingPassword),
    canConnect: existingPassword.length > 0 && !connect.isPending,
    isConnecting: connect.isPending,
    connectError: connect.error?.message ?? null,
    /** Retention, check and unlock: administrators, initialised repository. */
    canMaintain: canManage && (config?.initialized ?? false),
    runOperation: (op: 'forget-prune' | 'check') => operation.mutate(op),
    askUnlock: () => setConfirmUnlock(true),
    cancelUnlock: () => setConfirmUnlock(false),
    confirmUnlock,
    unlock: () => operation.mutate('unlock'),
    operationPending: operation.isPending ? (operation.variables ?? null) : null,
    operationDone: operation.isSuccess,
    operationError: operation.error?.message ?? null,
  }
}

export type BackupConfigFormModel = ReturnType<typeof useBackupConfigForm>
