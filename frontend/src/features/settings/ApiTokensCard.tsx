import { KeyRound, Plus } from 'lucide-react'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/DataTable'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { formatDateTime, formatRelativeTime } from '../../lib/format'
import { FormField, inputClass } from '../alerts/FormField'
import { useApiTokens, type ApiTokensModel } from './useApiTokens'

/** Read-only API tokens of the signed-in account, for scripts and dashboards. */
export function ApiTokensCard() {
  const model = useApiTokens()
  return (
    <Card>
      <CardHeader
        title="Jetons d'API"
        subtitle="Accès en lecture pour vos scripts (Home Assistant, Grafana…)"
        icon={<KeyRound className="h-3.5 w-3.5" />}
        actions={
          !model.formOpen && (
            <Button size="sm" icon={<Plus className="h-3.5 w-3.5" />} onClick={model.openForm}>
              Nouveau jeton
            </Button>
          )
        }
      />
      <CardBody className="space-y-3">
        <p className="text-xs text-text-secondary">
          Un jeton s&apos;envoie dans l&apos;en-tête <code>Authorization: Bearer hlm_…</code>. Il ne
          permet que la lecture des parties choisies, jamais une action, et cesse de fonctionner si
          votre compte est désactivé.
        </p>
        {model.secret && <SecretReveal model={model} />}
        {model.formOpen && <TokenForm model={model} />}
        {model.state.isLoading && <LoadingState label="Chargement des jetons…" rows={2} />}
        {model.state.isError && (
          <ErrorState message={model.state.error.message} onRetry={model.state.refetch} />
        )}
        {model.revokeError && (
          <p className="text-xs text-danger" role="alert">
            {model.revokeError}
          </p>
        )}
        {model.rows.length === 0 && !model.state.isLoading && !model.state.isError && (
          <p className="text-xs text-text-muted">Aucun jeton.</p>
        )}
        {model.rows.length > 0 && (
          <Table minWidthClassName="min-w-[720px]">
            <THead>
              <tr>
                <TH>Nom</TH>
                <TH>Portées</TH>
                <TH>Expiration</TH>
                <TH>Dernier usage</TH>
                <TH className="text-right">Révoquer</TH>
              </tr>
            </THead>
            <TBody>
              {model.rows.map((row) => (
                <TR key={row.id}>
                  <TD>
                    <p className="text-text-primary">{row.name}</p>
                    <p className="font-mono text-xs text-text-muted">{row.prefix}…</p>
                  </TD>
                  <TD className="text-xs">{row.scopes}</TD>
                  <TD>
                    <Badge tone={row.expiry.tone}>{row.expiry.label}</Badge>
                  </TD>
                  <TD className="text-xs" title={formatDateTime(row.lastUsedAt)}>
                    {row.lastUsedAt ? formatRelativeTime(row.lastUsedAt) : 'Jamais'}
                  </TD>
                  <TD className="text-right">
                    <Button
                      size="sm"
                      variant="danger"
                      loading={model.revokingId === row.id}
                      aria-label={`Révoquer le jeton ${row.name}`}
                      onClick={() => model.revoke(row.id)}
                    >
                      Révoquer
                    </Button>
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        )}
      </CardBody>
    </Card>
  )
}

function TokenForm({ model }: { model: ApiTokensModel }) {
  return (
    <form
      onSubmit={model.onSubmit}
      noValidate
      className="space-y-3 rounded-md border border-line bg-surface p-3"
    >
      <div className="grid gap-3 sm:grid-cols-2">
        <FormField id="token-name" label="Nom" error={model.errors.name?.message}>
          <input
            id="token-name"
            autoComplete="off"
            className={inputClass}
            {...model.register('name')}
          />
        </FormField>
        <FormField
          id="token-days"
          label="Validité (jours, 365 au plus)"
          error={model.errors.expiresInDays?.message}
        >
          <input
            id="token-days"
            type="number"
            min={1}
            max={365}
            className={inputClass}
            {...model.register('expiresInDays', { valueAsNumber: true })}
          />
        </FormField>
      </div>
      <fieldset className="min-w-0">
        <legend className="mb-1.5 text-xs text-text-secondary">Portées en lecture</legend>
        <div className="flex flex-wrap gap-x-4 gap-y-1.5">
          {model.readScopes.map((option) => (
            <label
              key={option.value}
              className="flex items-center gap-1.5 text-xs text-text-secondary"
            >
              <input type="checkbox" value={option.value} {...model.register('scopes')} />
              {option.label}
            </label>
          ))}
        </div>
        {model.actionScopes.length > 0 && (
          <>
            <p className="mb-1.5 mt-3 text-xs text-text-secondary">Portées d'action</p>
            <div className="flex flex-wrap gap-x-4 gap-y-1.5">
              {model.actionScopes.map((option) => (
                <label
                  key={option.value}
                  className="flex items-center gap-1.5 text-xs text-text-secondary"
                >
                  <input type="checkbox" value={option.value} {...model.register('scopes')} />
                  {option.label}
                </label>
              ))}
            </div>
            <p className="mt-1 text-xs text-text-muted">
              Un jeton d'action démarre, arrête ou redémarre ce que votre rôle permet ; chaque usage
              est inscrit au journal d'audit. Ne le confiez qu'à un outil de confiance.
            </p>
          </>
        )}
        {model.errors.scopes && (
          <p className="mt-1 text-xs text-danger">{model.errors.scopes.message}</p>
        )}
      </fieldset>
      {model.createError && (
        <p className="text-xs text-danger" role="alert">
          {model.createError}
        </p>
      )}
      <div className="flex gap-2">
        <Button type="submit" size="sm" variant="primary" loading={model.isCreating}>
          Créer le jeton
        </Button>
        <Button size="sm" onClick={model.closeForm}>
          Annuler
        </Button>
      </div>
    </form>
  )
}

function SecretReveal({ model }: { model: ApiTokensModel }) {
  return (
    <div className="space-y-2 rounded-lg border border-warning/40 bg-warning/5 p-3" role="alert">
      <p className="text-sm font-semibold text-text-primary">Votre nouveau jeton</p>
      <p className="text-xs text-text-secondary">
        Copiez-le maintenant : il ne sera plus jamais affiché.
      </p>
      <code className="block break-all rounded bg-surface px-3 py-2 font-mono text-sm text-text-primary">
        {model.secret}
      </code>
      <Button size="sm" onClick={model.dismissSecret}>
        J&apos;ai copié le jeton
      </Button>
    </div>
  )
}
