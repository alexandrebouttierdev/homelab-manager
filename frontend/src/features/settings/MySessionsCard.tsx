import { MonitorSmartphone } from 'lucide-react'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { SessionsTable } from '../sessions/SessionsTable'
import { useSessionsList } from '../sessions/useSessionsList'

/** Active sessions of the signed-in account. */
export function MySessionsCard() {
  const sessions = useSessionsList(false)

  return (
    <Card>
      <CardHeader
        title="Sessions actives"
        subtitle="Navigateurs connectés à votre compte"
        icon={<MonitorSmartphone className="h-3.5 w-3.5" />}
        actions={
          sessions.hasOthers && (
            <Button
              size="sm"
              variant="danger"
              loading={sessions.isRevokingOthers}
              onClick={sessions.revokeOthers}
            >
              Révoquer les autres sessions
            </Button>
          )
        }
      />
      <CardBody className="space-y-3">
        {sessions.state.isError && (
          <ErrorState message={sessions.state.error.message} onRetry={sessions.state.refetch} />
        )}
        {sessions.state.isLoading && <LoadingState label="Chargement des sessions…" rows={2} />}
        {sessions.errorMessage && (
          <p className="text-xs text-danger" role="alert">
            {sessions.errorMessage}
          </p>
        )}
        {sessions.othersRevoked !== null && (
          <p className="text-xs text-success" role="status">
            {sessions.othersRevoked} session(s) révoquée(s).
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
      </CardBody>
    </Card>
  )
}
