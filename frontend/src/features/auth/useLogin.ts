import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useLocation, useNavigate } from 'react-router-dom'
import { sessionQueryKey } from '../../hooks/useSession'
import { login } from '../../lib/api/auth'
import { loginSchema, type LoginValues } from '../../schemas/login'

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

  const mutation = useMutation({
    mutationFn: (values: LoginValues) => login(values.username, values.password),
    onSuccess: (result, values) => {
      rememberUsername(remember ? values.username : null)
      queryClient.setQueryData(sessionQueryKey, result)
      navigate('/')
    },
  })

  const state = location.state as { notice?: unknown } | null
  const errorMessage = mutation.error?.message ?? null

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
  }
}
