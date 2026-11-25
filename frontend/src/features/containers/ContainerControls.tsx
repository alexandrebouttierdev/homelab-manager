import { Pause, Play, RefreshCw, ScrollText, X } from 'lucide-react'
import type { BadgeTone } from '../../components/ui/Badge'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardFooter, CardHeader } from '../../components/ui/Card'
import { EmptyState } from '../../components/ui/EmptyState'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { SearchField } from '../../components/ui/SearchField'
import { SegmentedTabs } from '../../components/ui/SegmentedTabs'
import { Select } from '../../components/ui/Select'
import { cn } from '../../lib/cn'
import { logTailOptions, type LogStreamFilter } from '../../lib/containerActions'
import { formatDateTime, formatTime } from '../../lib/format'
import type { ContainerActionModel, ContainerLogsModel } from './useContainerControls'

const toneClasses: Record<BadgeTone, string> = {
  neutral: 'text-text-secondary',
  success: 'text-success',
  warning: 'text-warning',
  danger: 'text-danger',
  accent: 'text-accent',
  info: 'text-info',
}

const streamOptions: Array<{ value: LogStreamFilter; label: string }> = [
  { value: 'all', label: 'Tout' },
  { value: 'stdout', label: 'stdout' },
  { value: 'stderr', label: 'stderr' },
]

/** Start, stop and restart buttons, their confirmation and progress (D-5). */
export function ContainerActionPanel({ model }: { model: ContainerActionModel }) {
  if (!model.visible) return null
  return (
    <div className="space-y-2">
      {model.choices.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {model.choices.map((choice) => (
            <Button
              key={choice.verb}
              size="sm"
              variant={choice.danger ? 'danger' : 'secondary'}
              disabled={model.busy || model.pending !== null}
              onClick={() => model.choose(choice.verb)}
            >
              {choice.label}
            </Button>
          ))}
        </div>
      )}
      {model.disabledNotice && (
        <p className="text-[11px] text-text-muted">
          Actions désactivées dans la configuration du Manager (<code>docker_actions_enabled</code>)
          ; le proxy Docker doit aussi les autoriser.
        </p>
      )}
      {model.pending && (
        <div
          className="space-y-2 rounded-md border border-line bg-surface p-3"
          role="group"
          aria-label="Confirmer l'action"
        >
          <p className="text-xs text-text-secondary">{model.pending.question}</p>
          <div className="flex gap-2">
            <Button
              size="sm"
              variant={model.pending.danger ? 'danger' : 'primary'}
              loading={model.isSubmitting}
              onClick={model.confirm}
            >
              Confirmer : {model.pending.label}
            </Button>
            <Button size="sm" onClick={model.cancel} disabled={model.isSubmitting}>
              Annuler
            </Button>
          </div>
        </div>
      )}
      {model.errorMessage && (
        <p className="text-xs text-danger" role="alert">
          {model.errorMessage}
        </p>
      )}
      {model.progress && (
        <div className="flex items-start justify-between gap-2" role="status">
          <p className={cn('text-xs', toneClasses[model.progress.tone])}>
            {model.progress.message}
          </p>
          {!model.busy && (
            <button
              type="button"
              aria-label="Masquer le résultat"
              onClick={model.dismiss}
              className="rounded-md p-0.5 text-text-muted hover:bg-elevated hover:text-text-primary"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          )}
        </div>
      )}
    </div>
  )
}

/** End of the container log, with stream filter, search and refresh. */
export function ContainerLogsCard({ model }: { model: ContainerLogsModel }) {
  return (
    <Card>
      <CardHeader
        title="Journaux du conteneur"
        subtitle={`${model.tail} dernières lignes · lecture seule`}
        icon={<ScrollText className="h-3.5 w-3.5" />}
        actions={
          !model.removed && (
            <div className="flex items-center gap-1.5">
              <Button
                size="sm"
                variant="ghost"
                icon={
                  model.follow ? (
                    <Pause className="h-3.5 w-3.5" />
                  ) : (
                    <Play className="h-3.5 w-3.5" />
                  )
                }
                aria-pressed={model.follow}
                onClick={() => model.setFollow(!model.follow)}
              >
                {model.follow ? 'Suspendre' : 'Suivre'}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                icon={
                  <RefreshCw className={cn('h-3.5 w-3.5', model.isFetching && 'animate-spin')} />
                }
                onClick={model.state.refetch}
              >
                Relire
              </Button>
            </div>
          )
        }
      />
      <CardBody className="space-y-3">
        {model.removed ? (
          <EmptyState
            icon={ScrollText}
            title="Journal indisponible"
            description="Le conteneur a été supprimé de l'hôte Docker : son journal n'existe plus."
            className="border-0 bg-transparent py-8"
          />
        ) : (
          <>
            <div className="flex flex-wrap items-center gap-2">
              <SegmentedTabs
                options={streamOptions}
                value={model.stream}
                onChange={model.setStream}
                ariaLabel="Flux du journal"
              />
              <Select
                aria-label="Nombre de lignes"
                value={model.tail}
                onChange={(event) => model.setTail(Number(event.target.value))}
              >
                {logTailOptions.map((option) => (
                  <option key={option} value={option}>
                    {option} lignes
                  </option>
                ))}
              </Select>
              <SearchField
                value={model.search}
                onChange={model.setSearch}
                placeholder="Filtrer les lignes…"
                ariaLabel="Filtrer les lignes du journal"
                className="min-w-48 flex-1"
              />
            </div>
            {model.state.isLoading && <LoadingState label="Lecture du journal…" rows={3} />}
            {model.state.isError && (
              <ErrorState message={model.state.error.message} onRetry={model.state.refetch} />
            )}
            {!model.state.isLoading && !model.state.isError && model.lines.length === 0 && (
              <EmptyState
                icon={ScrollText}
                title={model.total === 0 ? 'Journal vide' : 'Aucune ligne ne correspond'}
                description={
                  model.total === 0
                    ? "Le conteneur n'a encore rien écrit sur stdout ni stderr."
                    : 'Changez le flux ou le filtre.'
                }
                className="border-0 bg-transparent py-8"
              />
            )}
            {model.lines.length > 0 && (
              <ol
                aria-label="Lignes du journal"
                className="max-h-[28rem] overflow-auto rounded-md border border-line bg-surface p-2 font-mono text-[11px] leading-relaxed"
              >
                {model.lines.map((line, index) => (
                  <li key={index} className="flex gap-2 whitespace-pre-wrap break-all">
                    <span
                      className="shrink-0 text-text-muted"
                      title={line.time ? formatDateTime(line.time) : undefined}
                    >
                      {line.time ? formatTime(line.time) : '--:--:--'}
                    </span>
                    <span
                      className={cn(
                        'min-w-0',
                        line.stream === 'stderr' ? 'text-warning' : 'text-text-primary',
                      )}
                    >
                      {line.message}
                      {line.cut && <span className="text-text-muted"> […]</span>}
                    </span>
                  </li>
                ))}
              </ol>
            )}
          </>
        )}
      </CardBody>
      {!model.removed && (
        <CardFooter className="flex flex-wrap items-center justify-between gap-2 font-mono text-[10px] text-text-muted">
          <span>
            {model.lines.length} / {model.total} ligne(s) affichée(s)
            {model.follow && ' · relu toutes les 5 s'}
          </span>
          {model.truncated && (
            <Badge tone="warning">Journal trop volumineux : lignes les plus anciennes omises</Badge>
          )}
        </CardFooter>
      )}
    </Card>
  )
}
