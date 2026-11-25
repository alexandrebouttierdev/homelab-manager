import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { useForm } from 'react-hook-form'
import { useLocation, useNavigate } from 'react-router-dom'
import { sessionQueryKey } from '../../hooks/useSession'
import {
  ApiError,
  beginPasskeyLogin,
  beginWebAuthnLogin,
  completePasskeyLogin,
  completeSecondFactor,
  completeWithKey,
  login,
  needsSecondFactor,
  passkeyLoginAvailable,
  type SecondFactorMethod,
  type SessionUser,
} from '../../lib/api/auth'
import { otherMethods, preferredMethod } from '../../lib/secondFactor'
import { getCredential, webauthnSupported } from '../../lib/webauthn'
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
  const [methods, setMethods] = useState<SecondFactorMethod[]>([])
  const [method, setMethod] = useState<SecondFactorMethod>('totp')
  const codeForm = useForm<SecondFactorLoginValues>({
    resolver: zodResolver(secondFactorLoginSchema),
    defaultValues: { method: 'totp', code: '' },
  })

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
        setMethods(result.methods)
        switchMethod(preferredMethod(result.methods))
        setChallenged(true)
        return
      }
      signedIn(result.user)
    },
  })
  // The pending login is gone: back to the password, with the reason.
  const expired = (error: Error) => {
    if (error instanceof ApiError && error.code === 'LOGIN_CHALLENGE_EXPIRED') {
      setChallenged(false)
      setExpiredMessage(error.message)
      return true
    }
    return false
  }
  const second = useMutation({
    mutationFn: (values: SecondFactorLoginValues) =>
      completeSecondFactor(values.method, values.code),
    onSuccess: (result) => signedIn(result.user),
    onError: (error) => {
      if (expired(error)) second.reset()
    },
  })
  const keyLogin = useMutation({
    mutationFn: async () => completeWithKey(await getCredential(await beginWebAuthnLogin())),
    onSuccess: (result) => signedIn(result.user),
    onError: (error) => {
      if (expired(error)) keyLogin.reset()
    },
  })
  // Login with a passkey alone, when the policy allows it (decision D-14).
  const passkeyStatus = useQuery({
    queryKey: ['auth', 'passkey-login'],
    queryFn: passkeyLoginAvailable,
    enabled: webauthnSupported(),
    retry: false,
  })
  const passkeyLogin = useMutation({
    mutationFn: async () => completePasskeyLogin(await getCredential(await beginPasskeyLogin())),
    onMutate: () => setExpiredMessage(null),
    onSuccess: (result) => signedIn(result.user),
  })
  const switchMethod = (next: SecondFactorMethod) => {
    setMethod(next)
    if (next !== 'webauthn') codeForm.reset({ method: next, code: '' })
    second.reset()
    keyLogin.reset()
  }

  const state = location.state as { notice?: unknown } | null
  const errorMessage = mutation.error?.message ?? passkeyLogin.error?.message ?? expiredMessage

  return {
    register: form.register,
    errors: form.formState.errors,
    onSubmit: form.handleSubmit((values) => mutation.mutate(values)),
    remember,
    setRemember,
    isPending: mutation.isPending || passkeyLogin.isPending,
    /** Login with a passkey alone: offered when the policy and the browser allow it. */
    passkey:
      passkeyStatus.data === true
        ? {
            /** Browsers require a click to ask a passkey. */
            onClick: () => {
              mutation.reset()
              passkeyLogin.mutate()
            },
            isPending: passkeyLogin.isPending,
          }
        : null,
    errorMessage,
    /** Message passed by the previous page (e.g. after a password change). */
    notice: typeof state?.notice === 'string' && !errorMessage ? state.notice : null,
    /** Second step, once the password is accepted for an account with a second factor. */
    secondFactor: challenged
      ? {
          method,
          /** The other methods of the account, to switch to. */
          alternatives: otherMethods(methods, method),
          switchTo: switchMethod,
          registerCode: codeForm.register,
          codeError: codeForm.formState.errors.code?.message ?? null,
          /** Sends the code, or asks the security key: browsers require a click for it. */
          onSubmit: (event: FormEvent<HTMLFormElement>) => {
            if (method !== 'webauthn')
              return codeForm.handleSubmit((values) => second.mutate(values))(event)
            event.preventDefault()
            keyLogin.mutate()
          },
          isPending: second.isPending || keyLogin.isPending,
          errorMessage: (second.error ?? keyLogin.error)?.message ?? null,
          restart: () => {
            setChallenged(false)
            second.reset()
            keyLogin.reset()
          },
        }
      : null,
  }
}
