import { useSession, useSignOut } from '../../hooks/useSession'
import { useSecondFactor } from '../settings/useSecondFactor'

/**
 * Enrollment imposed on an administrator when the policy requires a second
 * factor. The page stays until the recovery codes are acknowledged.
 */
export function useSecondFactorSetupPage() {
  const session = useSession()
  const secondFactor = useSecondFactor()
  const signOut = useSignOut()
  let redirectTo: '/login' | '/' | null = null
  if (!session.isPending) {
    if (session.unauthenticated) redirectTo = '/login'
    else if (!session.mustEnrollSecondFactor && !secondFactor.recoveryCodes) redirectTo = '/'
  }
  return {
    redirectTo,
    username: session.user?.username ?? null,
    secondFactor,
    signOut: signOut.signOut,
  }
}
