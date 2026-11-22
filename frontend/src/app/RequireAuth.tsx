import { Navigate } from 'react-router-dom'
import type { ReactNode } from 'react'
import { Logo } from '../components/ui/Logo'
import { useSession } from '../hooks/useSession'

export function RequireAuth({ children }: { children: ReactNode }) {
  const session = useSession()

  if (session.isPending) {
    return (
      <main className="flex min-h-screen flex-col items-center justify-center gap-4 bg-background">
        <Logo />
        <span
          className="inline-flex items-center gap-2 font-mono text-[11px] uppercase tracking-wider text-text-muted"
          role="status"
        >
          <span
            className="h-3 w-3 animate-spin rounded-full border border-line border-t-accent"
            aria-hidden="true"
          />
          Vérification de la session…
        </span>
      </main>
    )
  }

  if (session.unauthenticated) {
    return <Navigate to="/login" replace />
  }

  if (session.error) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-background px-4">
        <div className="w-full max-w-md rounded-lg border border-danger/30 bg-card p-6 text-center">
          <p className="text-sm font-medium text-text-primary">Manager indisponible</p>
          <p className="mt-2 text-sm text-danger" role="alert">
            {session.error.message}
          </p>
        </div>
      </main>
    )
  }

  return children
}
