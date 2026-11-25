import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import {
  connectBackupCopy,
  initBackupCopy,
  removeBackupCopy,
  requestBackupOperation,
  setBackupCopy,
  type BackupConfig,
  type BackupRun,
} from '../../lib/api/backups'
import {
  copyToValues,
  copyValuesToInput,
  credentialLabels,
  describeDetails,
  locationHint,
  repositoryLabel,
  repositoryTypeOptions,
  runFailureText,
  runStateInfo,
} from '../../lib/backups'
import { backupCopySchema, type BackupCopyValues } from '../../schemas/backupCopy'

const backupsQueryKey = ['backups'] as const

/**
 * The copy repository (restic copy, SPEC_ADMINISTRATION.md § 7.2): set by
 * an administrator, created or connected once, then copied to by hand or
 * after each scheduled backup.
 */
export function useBackupCopy(
  config: BackupConfig | undefined,
  lastCopy: BackupRun | null,
  canManage: boolean,
) {
  const queryClient = useQueryClient()
  const copy = config?.copy ?? null
  const [password, setPassword] = useState<string | null>(null)
  const [existingPassword, setExistingPassword] = useState('')
  const [confirmRemove, setConfirmRemove] = useState(false)
  const form = useForm<BackupCopyValues>({
    resolver: zodResolver(backupCopySchema),
    values: config ? copyToValues(copy) : undefined,
    resetOptions: { keepDirtyValues: true },
  })
  const keepConfig = (saved: BackupConfig) => {
    queryClient.setQueryData([...backupsQueryKey, 'config'], saved)
    form.reset(copyToValues(saved.copy))
  }
  const refresh = () => queryClient.invalidateQueries({ queryKey: backupsQueryKey })
  const save = useMutation({ mutationFn: setBackupCopy, onSuccess: keepConfig, onSettled: refresh })
  const remove = useMutation({
    mutationFn: removeBackupCopy,
    onSuccess: (saved) => {
      setConfirmRemove(false)
      keepConfig(saved)
    },
    onSettled: refresh,
  })
  const init = useMutation({
    mutationFn: initBackupCopy,
    onSuccess: (result) => setPassword(result.password),
    onSettled: refresh,
  })
  const connect = useMutation({
    mutationFn: connectBackupCopy,
    onSuccess: () => setExistingPassword(''),
    onSettled: refresh,
  })
  const copyNow = useMutation({
    mutationFn: () => requestBackupOperation('copy'),
    onSettled: refresh,
  })
  const type = useWatch({ control: form.control, name: 'type' })
  const mainReady = config?.initialized ?? false
  const dirty = form.formState.isDirty

  return {
    /** The section shows once the main repository is set. */
    visible: Boolean(config?.repository) && (canManage || copy !== null),
    canManage,
    copy: copy && {
      label: `${repositoryLabel(copy.repository.type)} · ${copy.repository.location}`,
      initialized: copy.initialized,
      initializedAt: copy.initializedAt,
      afterBackup: copy.afterBackup,
    },
    lastCopy: lastCopy && {
      startedAt: lastCopy.startedAt,
      state: runStateInfo(lastCopy),
      result: describeDetails(lastCopy),
      problem: runFailureText(lastCopy),
    },
    register: form.register,
    errors: form.formState.errors,
    typeOptions: repositoryTypeOptions.filter((option) => option.value !== 'none'),
    credentials: credentialLabels(type),
    locationHint: locationHint(type ?? 'local'),
    isSFTP: type === 'sftp',
    hostKeyFingerprint:
      copy?.repository.type === 'sftp' ? (copy.repository.hostKeyFingerprint ?? null) : null,
    usernameSet: copy?.credentials.username ?? false,
    secretSet: copy?.credentials.secret ?? false,
    onSubmit: form.handleSubmit((values) => save.mutate(copyValuesToInput(values))),
    isSaving: save.isPending,
    saved: save.isSuccess && !dirty,
    saveError: save.error?.message ?? null,
    secretsAvailable: config?.secretsAvailable ?? true,
    /** Created or connected once saved, the main repository initialized. */
    canInit: canManage && copy !== null && !copy.initialized && mainReady && !dirty,
    needsMain: copy !== null && !mainReady,
    init: () => init.mutate(),
    isInitializing: init.isPending,
    initError: init.error?.message ?? null,
    password,
    dismissPassword: () => setPassword(null),
    existingPassword,
    setExistingPassword,
    connect: () => connect.mutate(existingPassword),
    canConnect: existingPassword.length > 0 && !connect.isPending,
    isConnecting: connect.isPending,
    connectError: connect.error?.message ?? null,
    canCopy: canManage && copy !== null && copy.initialized && mainReady && !dirty,
    copyNow: () => copyNow.mutate(),
    isCopying: copyNow.isPending,
    copyStarted: copyNow.isSuccess,
    copyError: copyNow.error?.message ?? null,
    canRemove: canManage && copy !== null,
    confirmRemove,
    askRemove: () => setConfirmRemove(true),
    cancelRemove: () => setConfirmRemove(false),
    remove: () => remove.mutate(),
    isRemoving: remove.isPending,
    removeError: remove.error?.message ?? null,
  }
}

export type BackupCopyModel = ReturnType<typeof useBackupCopy>
