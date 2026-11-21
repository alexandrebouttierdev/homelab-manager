import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { loginSchema, type LoginValues } from '../../schemas/login'
import { ApiError, login } from '../../lib/api/auth'

export function LoginPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { register, handleSubmit, formState: { errors } } = useForm<LoginValues>({
    resolver: zodResolver(loginSchema),
  })
  const mutation = useMutation({
    mutationFn: (values: LoginValues) => login(values.username, values.password),
    onSuccess: (result) => {
      queryClient.setQueryData(['session'], result)
      navigate('/')
    },
  })

  function onSubmit(values: LoginValues) {
    mutation.mutate(values)
  }

  return (
    <main className="auth-shell">
      <section className="auth-card" aria-labelledby="login-title">
        <p className="eyebrow">Homelab Manager</p>
        <h1 id="login-title">Connexion</h1>
        <form onSubmit={handleSubmit(onSubmit)} noValidate>
          <label htmlFor="username">Identifiant</label>
          <input id="username" autoComplete="username" {...register('username')} />
          {errors.username && <span className="field-error">{errors.username.message}</span>}

          <label htmlFor="password">Mot de passe</label>
          <input id="password" type="password" autoComplete="current-password" {...register('password')} />
          {errors.password && <span className="field-error">{errors.password.message}</span>}

          {mutation.error instanceof ApiError && (
            <p className="field-error" role="alert">{mutation.error.message}</p>
          )}
          <button type="submit" disabled={mutation.isPending}>
            {mutation.isPending ? 'Connexion...' : 'Se connecter'}
          </button>
        </form>
      </section>
    </main>
  )
}
