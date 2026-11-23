import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { ApiError, getSession, logout } from '../lib/api/auth'
import { canManageSettings, canManageUsers, roleLabel } from '../lib/roles'

export const sessionQueryKey = ['session'] as const

/** Current session; a 401 means "not signed in", not an error to retry. */
export function useSession() {
  const session = useQuery({
    queryKey: sessionQueryKey,
    queryFn: getSession,
    retry: false,
    staleTime: 30_000,
  })
  const unauthenticated = session.error instanceof ApiError && session.error.status === 401
  const user = session.data?.user ?? null
  return {
    user,
    isPending: session.isPending,
    unauthenticated,
    /** Error other than "not signed in" (Manager unreachable, database down…). */
    error: session.isError && !unauthenticated ? session.error : null,
    /** The account must choose its own password before anything else. */
    mustChangePassword: user?.mustChangePassword ?? false,
    roleLabel: user ? roleLabel(user.role) : null,
    canManageUsers: canManageUsers(user?.role),
    canManageSettings: canManageSettings(user?.role),
  }
}

/** Ends the session and returns to the login page. */
export function useSignOut() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const mutation = useMutation({
    mutationFn: logout,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: sessionQueryKey })
      navigate('/login', { replace: true })
    },
  })
  return {
    signOut: () => mutation.mutate(),
    isPending: mutation.isPending,
    error: mutation.error,
  }
}
