import { Pencil, Plus, Send, Trash2 } from 'lucide-react'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/DataTable'
import { EmptyState } from '../../components/ui/EmptyState'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { Select } from '../../components/ui/Select'
import { FormField, inputClass } from './FormField'
import { useChannelsPanel, type ChannelsPanelModel } from './useChannelsPanel'

export function ChannelsPanel() {
  const page = useChannelsPanel()
  if (!page.canManage)
    return (
      <p className="text-xs text-text-secondary">
        Seuls les administrateurs configurent les canaux.
      </p>
    )

  return (
    <div className="space-y-4">
      <p className="text-xs text-text-secondary">
        Les notifications sont la seule communication sortante du Manager : un message par minute au
        plus et par canal, sans nouvel essai en cas d’échec. Les secrets sont chiffrés et ne sont
        jamais réaffichés.
      </p>
      {!page.secretsAvailable && (
        <p className="rounded-md border border-warning/40 p-3 text-xs text-warning" role="status">
          Clé du coffre absente : les canaux qui ont un secret ne peuvent ni être créés ni envoyer.
          Les canaux sans secret (ntfy public, webhook non signé) fonctionnent.
        </p>
      )}
      {page.actionError && (
        <p className="text-xs text-danger" role="alert">
          {page.actionError}
        </p>
      )}
      {page.testResult && (
        <p
          className={page.testResult.ok ? 'text-xs text-success' : 'text-xs text-danger'}
          role={page.testResult.ok ? 'status' : 'alert'}
        >
          {page.testResult.message}
        </p>
      )}
      {!page.editing && (
        <Button
          size="sm"
          variant="primary"
          icon={<Plus className="h-3.5 w-3.5" />}
          onClick={page.startCreate}
        >
          Nouveau canal
        </Button>
      )}
      {page.editing && <ChannelForm page={page} />}

      {page.pendingDeletion && (
        <Card className="border-danger/40">
          <CardBody className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm text-text-primary">
              Supprimer le canal « {page.pendingDeletion.name} » et ses secrets ?
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

      {page.state.isLoading && <LoadingState label="Chargement des canaux…" rows={2} />}
      {page.state.isError && (
        <ErrorState message={page.state.error.message} onRetry={page.state.refetch} />
      )}
      {!page.state.isLoading && !page.state.isError && page.rows.length === 0 && (
        <EmptyState
          icon={Send}
          title="Aucun canal"
          description="Ajoutez un canal pour recevoir les alertes sur Telegram, Discord, Gotify, ntfy, par e-mail ou par webhook."
        />
      )}
      {page.rows.length > 0 && (
        <Table>
          <THead>
            <TR>
              <TH>Canal</TH>
              <TH>Destination</TH>
              <TH>Secrets</TH>
              <TH>Dernier envoi</TH>
              <TH>Activation</TH>
              <TH className="relative">
                <span className="sr-only">Actions</span>
              </TH>
            </TR>
          </THead>
          <TBody>
            {page.rows.map((row) => (
              <TR key={row.id}>
                <TD>
                  <p className="text-text-primary">{row.name}</p>
                  <p className="text-xs text-text-muted">{row.type}</p>
                  <p className="text-xs text-text-muted">{row.routing}</p>
                </TD>
                <TD className="break-all text-xs">{row.destination}</TD>
                <TD className="text-xs">
                  {row.secrets.map((secret) => (
                    <p key={secret.name}>
                      {secret.label} : <span className="font-mono">{secret.text}</span>
                    </p>
                  ))}
                </TD>
                <TD className="text-xs">
                  <Badge tone={row.status.tone}>{row.status.label}</Badge>
                  <p className="mt-1 text-xs text-text-muted">Réussi : {row.lastSent}</p>
                  {row.lastError && <p className="text-xs text-danger">Échec : {row.lastError}</p>}
                </TD>
                <TD>
                  <Button
                    size="sm"
                    disabled={page.isToggling}
                    aria-pressed={row.enabled}
                    onClick={() => page.toggleEnabled(row.id)}
                  >
                    {row.enabled ? 'Actif' : 'Désactivé'}
                  </Button>
                </TD>
                <TD className="whitespace-nowrap text-right">
                  <div className="flex justify-end gap-1">
                    <Button
                      size="sm"
                      icon={<Send className="h-3.5 w-3.5" />}
                      loading={page.testingId === row.id}
                      onClick={() => page.test(row.id)}
                    >
                      Tester
                    </Button>
                    <Button
                      size="sm"
                      aria-label={`Modifier le canal ${row.name}`}
                      icon={<Pencil className="h-3.5 w-3.5" />}
                      onClick={() => page.startEdit(row.id)}
                    />
                    <Button
                      size="sm"
                      aria-label={`Supprimer le canal ${row.name}`}
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
    </div>
  )
}

function ChannelForm({ page }: { page: ChannelsPanelModel }) {
  const { register, errors, type, options } = page

  return (
    <Card>
      <CardHeader title={page.isNew ? 'Nouveau canal' : 'Modifier le canal'} />
      <CardBody>
        <form
          className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3"
          onSubmit={page.onSave}
          noValidate
        >
          <FormField id="channelName" label="Nom" error={errors.name?.message}>
            <input id="channelName" className={inputClass} {...register('name')} />
          </FormField>
          <FormField
            id="channelType"
            label="Type"
            hint={page.isNew ? undefined : 'Le type ne change pas : créez un autre canal.'}
          >
            {page.isNew ? (
              <Select id="channelType" containerClassName="w-full" {...register('type')}>
                {options.types.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </Select>
            ) : (
              <input id="channelType" readOnly className={inputClass} value={page.typeLabel} />
            )}
          </FormField>
          <FormField id="channelSeverity" label="Sévérité transmise">
            <Select id="channelSeverity" containerClassName="w-full" {...register('minSeverity')}>
              {options.severities.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </Select>
          </FormField>
          <FormField
            id="channelRenotify"
            label="Renotifier (minutes)"
            hint="0 : jamais ; sinon 5 à 1 440, tant que l’alerte n’est pas acquittée"
            error={errors.renotifyMinutes?.message}
          >
            <input
              id="channelRenotify"
              inputMode="numeric"
              className={inputClass}
              {...register('renotifyMinutes')}
            />
          </FormField>

          {type === 'telegram' && (
            <FormField
              id="channelChatId"
              label="Identifiant du chat"
              hint="Nombre (négatif pour un groupe) ou @canal"
              error={errors.chatId?.message}
            >
              <input id="channelChatId" className={inputClass} {...register('chatId')} />
            </FormField>
          )}
          {(type === 'gotify' || type === 'ntfy') && (
            <FormField
              id="channelServerUrl"
              label="URL du serveur"
              error={errors.serverUrl?.message}
            >
              <input
                id="channelServerUrl"
                className={inputClass}
                placeholder={type === 'ntfy' ? 'https://ntfy.sh' : 'http://192.168.1.10:8080'}
                {...register('serverUrl')}
              />
            </FormField>
          )}
          {type === 'ntfy' && (
            <FormField id="channelTopic" label="Sujet" error={errors.topic?.message}>
              <input id="channelTopic" className={inputClass} {...register('topic')} />
            </FormField>
          )}
          {type === 'smtp' && (
            <>
              <FormField id="channelHost" label="Serveur SMTP" error={errors.host?.message}>
                <input id="channelHost" className={inputClass} {...register('host')} />
              </FormField>
              <FormField id="channelPort" label="Port" error={errors.port?.message}>
                <input
                  id="channelPort"
                  inputMode="numeric"
                  className={inputClass}
                  {...register('port')}
                />
              </FormField>
              <FormField id="channelSecurity" label="Chiffrement">
                <Select id="channelSecurity" containerClassName="w-full" {...register('security')}>
                  <option value="starttls">STARTTLS (port 587)</option>
                  <option value="tls">TLS immédiat (port 465)</option>
                </Select>
              </FormField>
              <FormField
                id="channelUsername"
                label="Utilisateur"
                hint="Vide pour un serveur sans authentification"
                error={errors.username?.message}
              >
                <input id="channelUsername" className={inputClass} {...register('username')} />
              </FormField>
              <FormField id="channelFrom" label="Expéditeur" error={errors.from?.message}>
                <input id="channelFrom" className={inputClass} {...register('from')} />
              </FormField>
              <FormField
                id="channelTo"
                label="Destinataires"
                hint="1 à 10 adresses, une par ligne"
                error={errors.to?.message}
              >
                <textarea
                  id="channelTo"
                  rows={2}
                  className="w-full rounded-md border border-line bg-surface px-3 py-2 text-sm text-text-primary outline-none focus:border-accent"
                  {...register('to')}
                />
              </FormField>
            </>
          )}
          {type === 'webhook' && (
            <FormField
              id="channelUrl"
              label="URL du webhook"
              error={errors.url?.message}
              className="sm:col-span-2"
            >
              <input id="channelUrl" className={inputClass} {...register('url')} />
            </FormField>
          )}

          {page.secretFields.map((field) => (
            <div key={field.name}>
              <FormField
                id={`channelSecret-${field.name}`}
                label={field.label}
                hint={
                  field.defined
                    ? `Défini${field.fingerprint ? ` (${field.fingerprint})` : ''} : laisser vide pour le garder`
                    : field.hint
                }
                error={errors[field.name]?.message}
              >
                <input
                  id={`channelSecret-${field.name}`}
                  type="password"
                  autoComplete="off"
                  className={inputClass}
                  {...register(field.name)}
                />
              </FormField>
              {field.defined && !field.required && (
                <label className="mt-1 flex items-center gap-1.5 text-xs text-text-secondary">
                  <input type="checkbox" value={field.name} {...register('removeSecrets')} />
                  Supprimer ce secret
                </label>
              )}
            </div>
          ))}

          <label className="flex items-center gap-2 self-end text-sm">
            <input type="checkbox" {...register('enabled')} />
            Canal actif
          </label>

          <div className="flex flex-wrap items-center gap-2 sm:col-span-2 xl:col-span-3">
            <Button type="submit" size="sm" variant="primary" loading={page.isSaving}>
              Enregistrer
            </Button>
            <Button size="sm" onClick={page.cancelEdit}>
              Annuler
            </Button>
          </div>
          {page.saveError && (
            <p className="text-xs text-danger sm:col-span-2 xl:col-span-3" role="alert">
              {page.saveError}
            </p>
          )}
        </form>
      </CardBody>
    </Card>
  )
}
