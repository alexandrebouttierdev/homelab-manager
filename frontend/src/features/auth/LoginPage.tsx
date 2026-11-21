import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { loginSchema, type LoginValues } from '../../schemas/login'

export function LoginPage() {
  const { register, handleSubmit, formState: { errors } } = useForm<LoginValues>({
    resolver: zodResolver(loginSchema),
  })

  function onSubmit() {
    // API integration is intentionally deferred until authentication usecases exist.
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

          <button type="submit">Se connecter</button>
        </form>
      </section>
    </main>
  )
}
