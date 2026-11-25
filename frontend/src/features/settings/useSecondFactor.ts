import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { loadState } from '../../hooks/loadState'
import { sessionQueryKey } from '../../hooks/useSession'
import {
  beginTotp,
  beginWebAuthn,
  confirmTotp,
  confirmWebAuthn,
  deleteWebAuthn,
  disableSecondFactor,
  getSecondFactorStatus,
  regenerateRecoveryCodes,
  type WebAuthnKey,
} from '../../lib/api/secondFactor'
import { formatDateTime } from '../../lib/format'
import { qrPath, qrViewBox } from '../../lib/qr'
import { formatTotpSecret } from '../../lib/secondFactor'
import { createCredential, webauthnSupported } from '../../lib/webauthn'
import {
  passwordConfirmationSchema,
  totpCodeSchema,
  webauthnKeySchema,
  type PasswordConfirmationValues,
  type TotpCodeValues,
  type WebAuthnKeyValues,
} from '../../schemas/secondFactor'

export const secondFactorQueryKey = ['second-factor'] as const

/** Action that asks for the current password first. */
export type SecondFactorAction = 'enroll' | 'regenerate' | 'disable' | 'addKey' | 'removeKey'

/**
 * Second factor of the signed-in account: TOTP enrollment with a QR code,
 * passkeys and security keys, recovery codes shown once, regeneration and
 * deactivation.
 */
export function useSecondFactor() {
  const queryClient = useQueryClient()
  const status = useQuery({ queryKey: secondFactorQueryKey, queryFn: getSecondFactorStatus })
  const [action, setAction] = useState<SecondFactorAction | null>(null)
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null)
  const [copied, setCopied] = useState(false)
  const [removingKey, setRemovingKey] = useState<WebAuthnKey | null>(null)
  const [supported] = useState(webauthnSupported)
  const passwordForm = useForm<PasswordConfirmationValues>({
    resolver: zodResolver(passwordConfirmationSchema),
    defaultValues: { password: '' },
  })
  const codeForm = useForm<TotpCodeValues>({
    resolver: zodResolver(totpCodeSchema),
    defaultValues: { code: '' },
  })
  const keyForm = useForm<WebAuthnKeyValues>({
    resolver: zodResolver(webauthnKeySchema),
    defaultValues: { name: '', password: '' },
  })
  // The session is refreshed once the recovery codes are acknowledged: an
  // administrator forced to enroll would otherwise leave before seeing them.
  const refresh = () => queryClient.invalidateQueries({ queryKey: secondFactorQueryKey })
  const refreshWithSession = () =>
    Promise.all([refresh(), queryClient.invalidateQueries({ queryKey: sessionQueryKey })])
  const showCodes = (codes: string[]) => {
    setAction(null)
    setCopied(false)
    setRecoveryCodes(codes)
  }

  const begin = useMutation({
    mutationFn: (password: string) => beginTotp(password),
    onSuccess: () => passwordForm.reset(),
  })
  const confirm = useMutation({
    mutationFn: (code: string) => confirmTotp(code),
    onSuccess: async (codes) => {
      codeForm.reset()
      begin.reset()
      showCodes(codes)
      await refresh()
    },
  })
  const regenerate = useMutation({
    mutationFn: (password: string) => regenerateRecoveryCodes(password),
    onSuccess: async (codes) => {
      passwordForm.reset()
      showCodes(codes)
      await refresh()
    },
  })
  const disable = useMutation({
    mutationFn: (password: string) => disableSecondFactor(password),
    onSuccess: async () => {
      passwordForm.reset()
      setAction(null)
      await refreshWithSession()
    },
  })
  const addKey = useMutation({
    mutationFn: async ({ name, password }: WebAuthnKeyValues) =>
      confirmWebAuthn(name, await createCredential(await beginWebAuthn(password))),
    onSuccess: async ({ recoveryCodes: codes }) => {
      keyForm.reset()
      // The first second factor of the account comes with recovery codes.
      if (codes.length > 0) showCodes(codes)
      else setAction(null)
      await refresh()
    },
  })
  const removeKey = useMutation({
    mutationFn: ({ id, password }: { id: string; password: string }) =>
      deleteWebAuthn(id, password),
    onSuccess: async () => {
      passwordForm.reset()
      setRemovingKey(null)
      setAction(null)
      await refreshWithSession()
    },
  })
  const mutations = [begin, confirm, regenerate, disable, addKey, removeKey]

  const enrollment = useMemo(
    () =>
      begin.data
        ? {
            secret: formatTotpSecret(begin.data.secret),
            path: qrPath(begin.data.qr.rows),
            viewBox: qrViewBox(begin.data.qr.size),
          }
        : null,
    [begin.data],
  )

  const keys = useMemo(
    () =>
      (status.data?.keys ?? []).map((key) => ({
        key,
        createdAt: formatDateTime(key.createdAt),
        lastUsedAt: key.lastUsedAt ? formatDateTime(key.lastUsedAt) : 'jamais',
      })),
    [status.data],
  )
  const start = (next: SecondFactorAction) => {
    mutations.forEach((mutation) => mutation.reset())
    passwordForm.reset()
    codeForm.reset()
    keyForm.reset()
    setAction(next)
  }

  return {
    state: loadState(status),
    status: status.data ?? null,
    /** A TOTP or at least one key protects the account. */
    secured: Boolean(status.data && (status.data.enabled || status.data.keys.length > 0)),
    keys,
    /** Keys can be added: the Manager has a WebAuthn domain and the browser the API. */
    canAddKey: Boolean(status.data?.webauthnAvailable) && supported,
    webauthnSupported: supported,
    action,
    /** The key removed by the "removeKey" action. */
    removingKey,
    /** The current password is asked before the chosen action. */
    askPassword: action !== null && action !== 'addKey' && !(action === 'enroll' && enrollment),
    start,
    startRemove: (key: WebAuthnKey) => {
      start('removeKey')
      setRemovingKey(key)
    },
    cancel: () => {
      begin.reset()
      setRemovingKey(null)
      setAction(null)
    },
    registerPassword: passwordForm.register,
    passwordErrors: passwordForm.formState.errors,
    onPasswordSubmit: passwordForm.handleSubmit(({ password }) => {
      if (action === 'enroll') begin.mutate(password)
      else if (action === 'regenerate') regenerate.mutate(password)
      else if (action === 'disable') disable.mutate(password)
      else if (action === 'removeKey' && removingKey)
        removeKey.mutate({ id: removingKey.id, password })
    }),
    registerKey: keyForm.register,
    keyErrors: keyForm.formState.errors,
    onKeySubmit: keyForm.handleSubmit((values) => addKey.mutate(values)),
    enrollment,
    registerCode: codeForm.register,
    codeErrors: codeForm.formState.errors,
    onCodeSubmit: codeForm.handleSubmit(({ code }) => confirm.mutate(code)),
    /** Shown only once, until acknowledged. */
    recoveryCodes,
    copied,
    copyCodes: async () => {
      if (!recoveryCodes) return
      try {
        await navigator.clipboard.writeText(recoveryCodes.join('\n'))
        setCopied(true)
      } catch {
        // Clipboard unavailable (insecure context): the codes stay on screen.
      }
    },
    acknowledgeCodes: async () => {
      setRecoveryCodes(null)
      await queryClient.invalidateQueries({ queryKey: sessionQueryKey })
    },
    isPending: mutations.some((mutation) => mutation.isPending),
    errorMessage: mutations.find((mutation) => mutation.error)?.error?.message ?? null,
  }
}

export type SecondFactorModel = ReturnType<typeof useSecondFactor>
