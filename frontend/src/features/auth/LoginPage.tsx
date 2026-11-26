import {
  Eye,
  EyeOff,
  Fingerprint,
  KeyRound,
  LockKeyhole,
  LogIn,
  Moon,
  Server,
  ShieldCheck,
  Sun,
  UserRound,
} from 'lucide-react'
import { useState } from 'react'
import { Logo } from '../../components/ui/Logo'
import { StatusDot } from '../../components/ui/StatusDot'
import { useTheme } from '../../lib/theme'
import { APP_VERSION } from '../../lib/version'
import { SecondFactorStep } from './SecondFactorStep'
import { useLogin } from './useLogin'

export function LoginPage() {
  const { theme, toggleTheme } = useTheme()
  const [showPassword, setShowPassword] = useState(false)
  const {
    register,
    errors,
    onSubmit,
    remember,
    setRemember,
    isPending,
    passkey,
    errorMessage,
    notice,
    secondFactor,
  } = useLogin()

  return (
    <main className="relative flex min-h-screen flex-col overflow-hidden bg-background">
      <header className="relative z-10 flex h-14 items-center justify-between border-b border-line/70 bg-background/70 px-4 backdrop-blur-sm sm:px-6">
        <Logo />
        <div className="flex items-center gap-2">
          <span className="hidden items-center gap-1.5 rounded-md border border-line bg-card px-2 py-1 text-xs text-text-secondary sm:flex">
            <StatusDot tone="success" pulse label="Manager local disponible" />
            Manager local
          </span>
          <button
            type="button"
            onClick={toggleTheme}
            className="rounded-md border border-line bg-card p-1.5 text-text-secondary transition-colors hover:bg-elevated hover:text-text-primary"
            aria-label={theme === 'dark' ? 'Activer le thème clair' : 'Activer le thème sombre'}
          >
            {theme === 'dark' ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
          </button>
        </div>
      </header>

      <div className="relative z-10 flex flex-1 items-center justify-center px-4 py-10">
        <section
          className="w-full max-w-md rounded-xl border border-line bg-card p-6 shadow-sm sm:p-8"
          aria-labelledby="login-title"
        >
          <div className="flex flex-col items-center text-center">
            <span className="flex h-11 w-11 items-center justify-center rounded-lg border border-line bg-elevated text-text-primary">
              <Server className="h-5 w-5" aria-hidden="true" />
            </span>
            <h1
              id="login-title"
              className="mt-3 text-[26px] font-semibold tracking-[-0.025em] text-text-primary"
            >
              Connexion
            </h1>
            <p className="mt-1 text-xs text-text-muted">MonoOps · supervision locale</p>
          </div>

          {secondFactor ? (
            <SecondFactorStep step={secondFactor} />
          ) : (
            <form className="mt-6 space-y-4" onSubmit={onSubmit} noValidate>
              <div>
                <label
                  htmlFor="username"
                  className="mb-1.5 block text-xs font-medium text-text-secondary"
                >
                  Nom d'utilisateur
                </label>
                <div className="flex items-center gap-2 rounded-md border border-line bg-surface px-3 transition-colors focus-within:border-accent/60">
                  <UserRound className="h-4 w-4 shrink-0 text-text-muted" aria-hidden="true" />
                  <input
                    id="username"
                    autoComplete="username"
                    autoFocus
                    placeholder="administrateur"
                    className="h-10 w-full bg-transparent text-sm text-text-primary outline-none placeholder:text-text-muted"
                    {...register('username')}
                  />
                </div>
                {errors.username && (
                  <p className="mt-1 text-xs text-danger">{errors.username.message}</p>
                )}
              </div>

              <div>
                <label
                  htmlFor="password"
                  className="mb-1.5 block text-xs font-medium text-text-secondary"
                >
                  Mot de passe
                </label>
                <div className="flex items-center gap-2 rounded-md border border-line bg-surface px-3 transition-colors focus-within:border-accent/60">
                  <LockKeyhole className="h-4 w-4 shrink-0 text-text-muted" aria-hidden="true" />
                  <input
                    id="password"
                    type={showPassword ? 'text' : 'password'}
                    autoComplete="current-password"
                    placeholder="••••••••••••"
                    className="h-10 w-full bg-transparent text-sm text-text-primary outline-none placeholder:text-text-muted"
                    {...register('password')}
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword((visible) => !visible)}
                    className="rounded-sm p-0.5 text-text-muted transition-colors hover:text-text-primary"
                    aria-label={
                      showPassword ? 'Masquer le mot de passe' : 'Afficher le mot de passe'
                    }
                  >
                    {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                  </button>
                </div>
                {errors.password && (
                  <p className="mt-1 text-xs text-danger">{errors.password.message}</p>
                )}
              </div>

              <label className="flex cursor-pointer select-none items-center gap-2 text-xs text-text-secondary">
                <input
                  type="checkbox"
                  checked={remember}
                  onChange={(event) => setRemember(event.target.checked)}
                  className="h-3.5 w-3.5 rounded-sm border-line bg-surface accent-accent"
                />
                Se souvenir de mon identifiant
              </label>

              {notice && (
                <div
                  className="rounded-md border border-success/30 bg-success/10 px-3 py-2 text-xs text-success"
                  role="status"
                >
                  {notice}
                </div>
              )}
              {errorMessage && (
                <div
                  className="rounded-md border border-danger/30 bg-danger/10 px-3 py-2 text-xs text-danger"
                  role="alert"
                >
                  {errorMessage}
                </div>
              )}

              <button
                type="submit"
                disabled={isPending}
                className="flex h-10 w-full items-center justify-center gap-2 rounded-md bg-accent px-4 text-sm font-semibold text-on-accent transition-colors hover:bg-accent/85 disabled:cursor-wait disabled:opacity-60"
              >
                {isPending ? (
                  <span
                    className="h-4 w-4 animate-spin rounded-full border-2 border-on-accent/30 border-t-on-accent"
                    aria-hidden="true"
                  />
                ) : (
                  <LogIn className="h-4 w-4" aria-hidden="true" />
                )}
                {isPending ? 'Connexion…' : 'Se connecter'}
              </button>
              {passkey && (
                <button
                  type="button"
                  onClick={passkey.onClick}
                  disabled={isPending}
                  className="flex h-10 w-full items-center justify-center gap-2 rounded-md border border-line bg-card px-4 text-sm font-medium text-text-primary transition-colors hover:bg-elevated disabled:cursor-wait disabled:opacity-60"
                >
                  <Fingerprint className="h-4 w-4" aria-hidden="true" />
                  {passkey.isPending
                    ? 'En attente de la passkey…'
                    : 'Se connecter avec une passkey'}
                </button>
              )}
            </form>
          )}

          <div className="mt-6 flex items-start gap-2 rounded-md border border-line/70 bg-surface/60 px-3 py-2.5">
            <ShieldCheck className="mt-0.5 h-3.5 w-3.5 shrink-0 text-success" aria-hidden="true" />
            <p className="text-xs leading-relaxed text-text-muted">
              Authentification locale. La session est protégée par un cookie HTTP-only et aucune
              donnée ne quitte cet hôte.
            </p>
          </div>

          <div className="mt-5 flex items-center justify-center gap-1.5 text-xs text-text-muted">
            <KeyRound className="h-3 w-3" aria-hidden="true" />
            Accès administrateur local
          </div>
        </section>
      </div>

      <footer className="relative z-10 flex flex-col items-center justify-between gap-2 border-t border-line/60 bg-background/60 px-4 py-3 text-xs text-text-muted sm:flex-row sm:px-6">
        <span>MonoOps · supervision mono-hôte</span>
        <span className="flex items-center gap-2">
          <span>
            {typeof window !== 'undefined' && window.location.protocol === 'https:'
              ? 'TLS actif'
              : 'HTTP local'}
          </span>
          <span className="text-line-strong">•</span>
          <span>v{APP_VERSION}</span>
        </span>
      </footer>
    </main>
  )
}
