import { useSession } from '../../hooks/useSession'
import { useChangePassword } from '../settings/useChangePassword'

/**
 * Password change imposed on an account created by an administrator. Once
 * done, the Manager revokes every session and the user signs in again.
 */
export function useForcedPasswordChange() {
  const session = useSession()
  const password = useChangePassword()
  let redirectTo: '/login' | '/' | null = null
  if (!session.isPending) {
    if (session.unauthenticated) redirectTo = '/login'
    else if (!session.mustChangePassword) redirectTo = '/'
  }
  return {
    checking: session.isPending,
    redirectTo,
    username: session.user?.username ?? null,
    register: password.register,
    errors: password.errors,
    onSubmit: password.onSubmit,
    isSubmitting: password.isPending,
    errorMessage: password.errorMessage,
  }
}
