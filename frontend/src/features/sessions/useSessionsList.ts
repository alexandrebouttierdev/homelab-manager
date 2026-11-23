import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo } from 'react'
import { loadState } from '../../hooks/loadState'
import { sessionQueryKey } from '../../hooks/useSession'
import { listSessions, revokeOtherSessions, revokeSession } from '../../lib/api/security'
import { toSessionRow } from '../../lib/sessions'

/**
 * Active sessions of the signed-in account, or of every account (all, for
 * administrators). Revoking the current session signs out: the session
 * query is refreshed and the login page takes over.
 */
export function useSessionsList(all: boolean) {
  const queryClient = useQueryClient()
  const sessions = useQuery({
    queryKey: ['sessions', all ? 'all' : 'mine'],
    queryFn: () => listSessions(all),
  })
  const refresh = () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: ['sessions'] }),
      queryClient.invalidateQueries({ queryKey: sessionQueryKey }),
    ])
  const revoke = useMutation({ mutationFn: (id: string) => revokeSession(id), onSettled: refresh })
  const revokeOthers = useMutation({ mutationFn: () => revokeOtherSessions(), onSettled: refresh })
  const rows = useMemo(() => (sessions.data ?? []).map(toSessionRow), [sessions.data])

  return {
    state: loadState(sessions),
    rows,
    showUser: all,
    hasOthers: rows.some((row) => !row.current),
    revoke: (id: string) => revoke.mutate(id),
    isRevoking: revoke.isPending,
    revokeOthers: () => revokeOthers.mutate(),
    isRevokingOthers: revokeOthers.isPending,
    othersRevoked: revokeOthers.data ?? null,
    errorMessage: revoke.error?.message ?? revokeOthers.error?.message ?? null,
  }
}
