import { useQuery } from '@tanstack/react-query'
import { Navigate } from 'react-router-dom'
import type { ReactNode } from 'react'
import { getSession } from '../lib/api/auth'

export function RequireAuth({ children }: { children: ReactNode }) {
  const session = useQuery({
    queryKey: ['session'],
    queryFn: getSession,
    retry: false,
    staleTime: 30_000,
  })

  if (session.isPending) return <main className="auth-shell"><p>Vérification de la session...</p></main>
  if (session.isError && 'status' in session.error && session.error.status === 401) {
    return <Navigate to="/login" replace />
  }
  if (session.isError) {
    return <main className="auth-shell"><p className="field-error" role="alert">{session.error.message}</p></main>
  }
  return children
}
