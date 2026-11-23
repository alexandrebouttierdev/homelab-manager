import { Trash2, UserPlus } from 'lucide-react'
import type { ReactNode } from 'react'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/DataTable'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { PageHeader } from '../../components/ui/PageHeader'
import { Select } from '../../components/ui/Select'
import { useUsersPage } from './useUsersPage'

const inputClass =
  'h-9 w-full rounded-md border border-line bg-surface px-3 text-sm text-text-primary outline-none focus:border-accent'

export function UsersPage() {
  const page = useUsersPage()

  return (
    <div className="space-y-4">
      <PageHeader
        title="Utilisateurs & accès"
        breadcrumb={[{ label: 'Paramètres' }, { label: 'Utilisateurs & accès' }]}
        description="Comptes de Homelab Manager uniquement : aucun compte Linux de la machine n'est lu ni modifié."
      />

      {page.state.isError && (
        <ErrorState message={page.state.error.message} onRetry={page.state.refetch} />
      )}
      {page.state.isLoading && <LoadingState label="Chargement des comptes…" rows={3} />}
      {page.actionError && (
        <p className="text-xs text-danger" role="alert">
          {page.actionError}
        </p>
      )}

      {page.pendingDeletion && (
        <Card className="border-danger/40">
          <CardBody className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm text-text-primary">
              Supprimer le compte « {page.pendingDeletion.username} » ? Ses sessions sont révoquées
              ; ses lignes d'audit sont conservées.
            </p>
            <div className="flex gap-2">
              <Button size="sm" onClick={page.cancelDelete}>
                Annuler
              </Button>
              <Button
                size="sm"
                variant="danger"
                loading={page.isDeleting}
                onClick={page.confirmDelete}
              >
                Confirmer la suppression
              </Button>
            </div>
          </CardBody>
        </Card>
      )}

      {page.rows.length > 0 && (
        <Table>
          <THead>
            <TR>
              <TH>Identifiant</TH>
              <TH>Nom affiché</TH>
              <TH>Rôle</TH>
              <TH>État</TH>
              <TH>Dernière connexion</TH>
              {/* relative: keeps the visually hidden label inside the scrolling table */}
              <TH className="relative">
                <span className="sr-only">Actions</span>
              </TH>
            </TR>
          </THead>
          <TBody>
            {page.rows.map((row) => (
              <TR key={row.id}>
                <TD className="font-mono text-text-primary">
                  {row.username}
                  {row.isSelf && <span className="ml-2 text-[10px] text-text-muted">(vous)</span>}
                </TD>
                <TD>{row.displayName}</TD>
                <TD>
                  <Select
                    aria-label={`Rôle de ${row.username}`}
                    value={row.role}
                    disabled={page.isUpdating}
                    onChange={(event) => page.changeRole(row.id, event.target.value)}
                  >
                    {page.roleOptions.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </Select>
                </TD>
                <TD>
                  <Badge tone={row.statusTone} dot>
                    {row.statusLabel}
                  </Badge>
                </TD>
                <TD className="font-mono text-[11px]">{row.lastLogin}</TD>
                <TD>
                  <div className="flex justify-end gap-2">
                    <Button
                      size="sm"
                      variant="ghost"
                      aria-label={`${row.toggleLabel} ${row.username}`}
                      disabled={page.isUpdating}
                      onClick={() => page.toggleDisabled(row)}
                    >
                      {row.toggleLabel}
                    </Button>
                    <Button
                      size="sm"
                      variant="danger"
                      aria-label={`Supprimer ${row.username}`}
                      icon={<Trash2 className="h-3.5 w-3.5" />}
                      onClick={() => page.askDelete(row)}
                    />
                  </div>
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      )}

      <Card>
        <CardHeader
          title="Nouveau compte"
          subtitle="Le mot de passe initial devra être changé à la première connexion"
          icon={<UserPlus className="h-3.5 w-3.5" />}
        />
        <CardBody>
          <form
            className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4"
            onSubmit={page.onCreate}
            noValidate
          >
            <Field id="username" label="Identifiant" error={page.errors.username?.message}>
              <input
                id="username"
                autoComplete="off"
                className={inputClass}
                {...page.register('username')}
              />
            </Field>
            <Field id="displayName" label="Nom affiché" error={page.errors.displayName?.message}>
              <input
                id="displayName"
                autoComplete="off"
                className={inputClass}
                {...page.register('displayName')}
              />
            </Field>
            <Field id="role" label="Rôle" error={page.errors.role?.message}>
              <Select id="role" containerClassName="w-full" {...page.register('role')}>
                {page.roleOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </Select>
            </Field>
            <Field id="password" label="Mot de passe initial" error={page.errors.password?.message}>
              <input
                id="password"
                type="password"
                autoComplete="new-password"
                className={inputClass}
                {...page.register('password')}
              />
            </Field>
            <div className="flex flex-wrap items-center gap-3 sm:col-span-2 lg:col-span-4">
              <Button
                type="submit"
                variant="primary"
                loading={page.isCreating}
                icon={<UserPlus className="h-3.5 w-3.5" />}
              >
                Créer le compte
              </Button>
              {page.createError && (
                <p className="text-xs text-danger" role="alert">
                  {page.createError}
                </p>
              )}
              {page.created && (
                <p className="text-xs text-success" role="status">
                  Compte créé. Transmettez le mot de passe initial à son titulaire.
                </p>
              )}
            </div>
          </form>
        </CardBody>
      </Card>
    </div>
  )
}

function Field({
  id,
  label,
  error,
  children,
}: {
  id: string
  label: string
  error?: string
  children: ReactNode
}) {
  return (
    <div>
      <label
        htmlFor={id}
        className="mb-1.5 block font-mono text-[10px] uppercase tracking-wider text-text-secondary"
      >
        {label}
      </label>
      {children}
      {error && (
        <p className="mt-1 text-[11px] text-danger" role="alert">
          {error}
        </p>
      )}
    </div>
  )
}
