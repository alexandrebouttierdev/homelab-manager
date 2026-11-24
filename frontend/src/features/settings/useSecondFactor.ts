import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { loadState } from '../../hooks/loadState'
import { sessionQueryKey } from '../../hooks/useSession'
import {
  beginTotp,
  confirmTotp,
  disableSecondFactor,
  getSecondFactorStatus,
  regenerateRecoveryCodes,
} from '../../lib/api/secondFactor'
import { qrPath, qrViewBox } from '../../lib/qr'
import { formatTotpSecret } from '../../lib/secondFactor'
import {
  passwordConfirmationSchema,
  totpCodeSchema,
  type PasswordConfirmationValues,
  type TotpCodeValues,
} from '../../schemas/secondFactor'

export const secondFactorQueryKey = ['second-factor'] as const

/** Action that asks for the current password first. */
export type SecondFactorAction = 'enroll' | 'regenerate' | 'disable'

/**
 * Second factor of the signed-in account: TOTP enrollment with a QR code,
 * recovery codes shown once, regeneration and deactivation.
 */
export function useSecondFactor() {
  const queryClient = useQueryClient()
  const status = useQuery({ queryKey: secondFactorQueryKey, queryFn: getSecondFactorStatus })
  const [action, setAction] = useState<SecondFactorAction | null>(null)
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null)
  const [copied, setCopied] = useState(false)
  const passwordForm = useForm<PasswordConfirmationValues>({
    resolver: zodResolver(passwordConfirmationSchema),
    defaultValues: { password: '' },
  })
  const codeForm = useForm<TotpCodeValues>({
    resolver: zodResolver(totpCodeSchema),
    defaultValues: { code: '' },
  })
  const refresh = () => queryClient.invalidateQueries({ queryKey: secondFactorQueryKey })
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
      await Promise.all([refresh(), queryClient.invalidateQueries({ queryKey: sessionQueryKey })])
    },
  })
  const mutations = [begin, confirm, regenerate, disable]

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

  return {
    state: loadState(status),
    status: status.data ?? null,
    action,
    /** The current password is asked before the chosen action. */
    askPassword: action !== null && !(action === 'enroll' && enrollment),
    start: (next: SecondFactorAction) => {
      mutations.forEach((mutation) => mutation.reset())
      passwordForm.reset()
      codeForm.reset()
      setAction(next)
    },
    cancel: () => {
      begin.reset()
      setAction(null)
    },
    registerPassword: passwordForm.register,
    passwordErrors: passwordForm.formState.errors,
    onPasswordSubmit: passwordForm.handleSubmit(({ password }) => {
      if (action === 'enroll') begin.mutate(password)
      else if (action === 'regenerate') regenerate.mutate(password)
      else if (action === 'disable') disable.mutate(password)
    }),
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
