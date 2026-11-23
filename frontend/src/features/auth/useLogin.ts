import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { useLocation, useNavigate } from 'react-router-dom'
import { sessionQueryKey } from '../../hooks/useSession'
import {
  ApiError,
  completeSecondFactor,
  login,
  needsSecondFactor,
  type SessionUser,
} from '../../lib/api/auth'
import { loginSchema, type LoginValues } from '../../schemas/login'
import { secondFactorLoginSchema, type SecondFactorLoginValues } from '../../schemas/secondFactor'

const usernameStorageKey = 'homelab-manager-username'

function readRememberedUsername(): string {
  try {
    return window.localStorage.getItem(usernameStorageKey) ?? ''
  } catch {
    return ''
  }
}

function rememberUsername(username: string | null) {
  try {
    if (username) window.localStorage.setItem(usernameStorageKey, username)
    else window.localStorage.removeItem(usernameStorageKey)
  } catch {
    // Storage unavailable (private mode): the username is simply not kept.
  }
}

/** Login form: validation, submission, remembered username and notices. */
export function useLogin() {
  const navigate = useNavigate()
  const location = useLocation()
  const queryClient = useQueryClient()
  const [rememberedUsername] = useState(readRememberedUsername)
  const [remember, setRemember] = useState(Boolean(rememberedUsername))

  const form = useForm<LoginValues>({
    resolver: zodResolver(loginSchema),
    defaultValues: { username: rememberedUsername, password: '' },
  })

  const [challenged, setChallenged] = useState(false)
  const [expiredMessage, setExpiredMessage] = useState<string | null>(null)
  const codeForm = useForm<SecondFactorLoginValues>({
    resolver: zodResolver(secondFactorLoginSchema),
    defaultValues: { method: 'totp', code: '' },
  })
  const method = useWatch({ control: codeForm.control, name: 'method' })

  const signedIn = (user: SessionUser) => {
    queryClient.setQueryData(sessionQueryKey, { user })
    navigate('/')
  }

  const mutation = useMutation({
    mutationFn: (values: LoginValues) => login(values.username, values.password),
    onMutate: () => setExpiredMessage(null),
    onSuccess: (result, values) => {
      rememberUsername(remember ? values.username : null)
      if (needsSecondFactor(result)) {
        codeForm.reset({ method: 'totp', code: '' })
        setChallenged(true)
        return
      }
      signedIn(result.user)
    },
  })
  const second = useMutation({
    mutationFn: (values: SecondFactorLoginValues) =>
      completeSecondFactor(values.method, values.code),
    onSuccess: (result) => signedIn(result.user),
    onError: (error) => {
      // The pending login is gone: back to the password, with the reason.
      if (error instanceof ApiError && error.code === 'LOGIN_CHALLENGE_EXPIRED') {
        setChallenged(false)
        setExpiredMessage(error.message)
        second.reset()
      }
    },
  })
  const switchMethod = (next: SecondFactorLoginValues['method']) => {
    codeForm.reset({ method: next, code: '' })
    second.reset()
  }

  const state = location.state as { notice?: unknown } | null
  const errorMessage = mutation.error?.message ?? expiredMessage

  return {
    register: form.register,
    errors: form.formState.errors,
    onSubmit: form.handleSubmit((values) => mutation.mutate(values)),
    remember,
    setRemember,
    isPending: mutation.isPending,
    errorMessage,
    /** Message passed by the previous page (e.g. after a password change). */
    notice: typeof state?.notice === 'string' && !errorMessage ? state.notice : null,
    /** Second step, once the password is accepted for an account with a second factor. */
    secondFactor: challenged
      ? {
          method,
          useRecoveryCode: () => switchMethod('recovery'),
          useApp: () => switchMethod('totp'),
          registerCode: codeForm.register,
          codeError: codeForm.formState.errors.code?.message ?? null,
          onSubmit: codeForm.handleSubmit((values) => second.mutate(values)),
          isPending: second.isPending,
          errorMessage: second.error?.message ?? null,
          restart: () => {
            setChallenged(false)
            second.reset()
          },
        }
      : null,
  }
}
