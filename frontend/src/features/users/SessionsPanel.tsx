import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { SessionsTable } from '../sessions/SessionsTable'
import { useSessionsList } from '../sessions/useSessionsList'

/** Every active session, for administrators. */
export function SessionsPanel() {
  const sessions = useSessionsList(true)

  return (
    <div className="space-y-3">
      {sessions.state.isError && (
        <ErrorState message={sessions.state.error.message} onRetry={sessions.state.refetch} />
      )}
      {sessions.state.isLoading && <LoadingState label="Chargement des sessions…" rows={3} />}
      {sessions.errorMessage && (
        <p className="text-xs text-danger" role="alert">
          {sessions.errorMessage}
        </p>
      )}
      {sessions.rows.length > 0 && (
        <SessionsTable
          rows={sessions.rows}
          showUser={sessions.showUser}
          onRevoke={sessions.revoke}
          revoking={sessions.isRevoking}
        />
      )}
      <p className="text-xs text-text-muted">
        Une session expire après 12 h, ou plus tôt si elle reste inactive au-delà de la durée de la
        politique. Un onglet ouvert, qui se rafraîchit seul, compte comme une activité.
      </p>
    </div>
  )
}
