import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { loadState } from '../../hooks/loadState'
import { useSession } from '../../hooks/useSession'
import { toAccountRow, type AccountRow } from '../../lib/accounts'
import {
  createAccount,
  deleteAccount,
  listAccounts,
  resetAccountPassword,
  updateAccount,
  type AccountChange,
} from '../../lib/api/users'
import { isRole, roleOptions } from '../../lib/roles'
import {
  accountSchema,
  passwordResetSchema,
  type AccountValues,
  type PasswordResetValues,
} from '../../schemas/account'

export const accountsQueryKey = ['accounts'] as const

/** Logic and view model of the accounts tab of UsersPage (administrators only). */
export function useAccountsPanel() {
  const queryClient = useQueryClient()
  const { user } = useSession()
  const accounts = useQuery({ queryKey: accountsQueryKey, queryFn: listAccounts })
  const refresh = () => queryClient.invalidateQueries({ queryKey: accountsQueryKey })

  const form = useForm<AccountValues>({
    resolver: zodResolver(accountSchema),
    defaultValues: { username: '', displayName: '', role: 'viewer', password: '' },
  })
  const create = useMutation({
    mutationFn: (values: AccountValues) => createAccount(values),
    onSuccess: async () => {
      form.reset()
      await refresh()
    },
  })
  const update = useMutation({
    mutationFn: ({ id, change }: { id: string; change: AccountChange }) =>
      updateAccount(id, change),
    onSettled: refresh,
  })
  const [pendingDeletion, setPendingDeletion] = useState<AccountRow | null>(null)
  const remove = useMutation({
    mutationFn: (id: string) => deleteAccount(id),
    onSuccess: () => setPendingDeletion(null),
    onSettled: refresh,
  })

  const [pendingReset, setPendingReset] = useState<AccountRow | null>(null)
  const resetForm = useForm<PasswordResetValues>({
    resolver: zodResolver(passwordResetSchema),
    defaultValues: { password: '' },
  })
  const reset = useMutation({
    mutationFn: ({ id, password }: { id: string; password: string }) =>
      resetAccountPassword(id, password),
    onSuccess: () => {
      resetForm.reset()
      setPendingReset(null)
    },
    onSettled: refresh,
  })

  const rows = useMemo(
    () => (accounts.data ?? []).map((account) => toAccountRow(account, user?.id)),
    [accounts.data, user?.id],
  )

  return {
    state: loadState(accounts),
    rows,
    roleOptions,
    register: form.register,
    errors: form.formState.errors,
    onCreate: form.handleSubmit((values) => create.mutate(values)),
    isCreating: create.isPending,
    created: create.isSuccess,
    createError: create.error?.message ?? null,
    changeRole: (id: string, value: string) => {
      if (isRole(value)) update.mutate({ id, change: { role: value } })
    },
    toggleDisabled: (row: AccountRow) =>
      update.mutate({ id: row.id, change: { disabled: !row.disabled } }),
    isUpdating: update.isPending,
    actionError: update.error?.message ?? remove.error?.message ?? null,
    pendingDeletion,
    askDelete: (row: AccountRow) => setPendingDeletion(row),
    cancelDelete: () => setPendingDeletion(null),
    confirmDelete: () => {
      if (pendingDeletion) remove.mutate(pendingDeletion.id)
    },
    isDeleting: remove.isPending,
    pendingReset,
    askReset: (row: AccountRow) => {
      reset.reset()
      resetForm.reset()
      setPendingReset(row)
    },
    cancelReset: () => setPendingReset(null),
    registerReset: resetForm.register,
    resetErrors: resetForm.formState.errors,
    onReset: resetForm.handleSubmit((values) => {
      if (pendingReset) reset.mutate({ id: pendingReset.id, password: values.password })
    }),
    isResetting: reset.isPending,
    resetError: reset.error?.message ?? null,
    /** Username whose password was just reset. */
    resetDone: reset.isSuccess && reset.variables ? rowName(rows, reset.variables.id) : null,
  }
}

function rowName(rows: AccountRow[], id: string): string | null {
  return rows.find((row) => row.id === id)?.username ?? null
}
