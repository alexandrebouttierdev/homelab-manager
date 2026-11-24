import { Play, X } from 'lucide-react'
import type { BadgeTone } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { cn } from '../../lib/cn'
import type { UnitActionChoice } from '../../lib/unitActions'
import type { UnitActionModel } from './useUnitAction'

const toneClasses: Record<BadgeTone, string> = {
  neutral: 'text-text-secondary',
  success: 'text-success',
  warning: 'text-warning',
  danger: 'text-danger',
  accent: 'text-accent',
  info: 'text-info',
}

/** Buttons of the actions offered on the unit shown; a choice on another unit runs a timer's service. */
export function UnitActionButtons({
  model,
  choices,
  unitName,
}: {
  model: UnitActionModel
  choices: UnitActionChoice[]
  unitName: string
}) {
  if (choices.length === 0) return null
  return (
    <div className="flex flex-wrap gap-1.5">
      {choices.map((choice) => (
        <Button
          key={`${choice.unit}:${choice.verb}`}
          size="sm"
          variant={choice.verb === 'stop' ? 'danger' : 'secondary'}
          icon={choice.unit !== unitName ? <Play className="h-3.5 w-3.5" /> : undefined}
          disabled={model.busy || model.pending !== null}
          onClick={() => model.choose(choice, choice.unit !== unitName ? unitName : undefined)}
        >
          {choice.label}
        </Button>
      ))}
    </div>
  )
}

/** Confirmation, request error and progress of a unit action. */
export function UnitActionStatus({ model }: { model: UnitActionModel }) {
  return (
    <>
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
    </>
  )
}
