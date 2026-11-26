import type { BadgeTone } from '../components/ui/Badge'
import type {
  ContainerAction,
  ContainerActionFailure,
  ContainerLogLine,
  ContainerVerb,
} from './api/containers'

const verbTexts: Record<ContainerVerb, { label: string; progress: string; done: string }> = {
  start: { label: 'Démarrer', progress: 'Démarrage', done: 'démarré' },
  stop: { label: 'Arrêter', progress: 'Arrêt', done: 'arrêté' },
  restart: { label: 'Redémarrer', progress: 'Redémarrage', done: 'redémarré' },
}

export function containerVerbLabel(verb: ContainerVerb): string {
  return verbTexts[verb].label
}

/**
 * Actions offered on a container (D-5): none while disabled or once
 * removed; Démarrer when it is not running, Redémarrer and Arrêter when it
 * is. The server checks again.
 */
export function containerActionChoices(
  state: string,
  removed: boolean,
  enabled: boolean,
): ContainerVerb[] {
  if (!enabled || removed) return []
  return state === 'running' || state === 'restarting' || state === 'paused'
    ? ['restart', 'stop']
    : ['start']
}

/** The question asked before an action: it names the container. */
export function containerActionQuestion(verb: ContainerVerb, name: string): string {
  const consequence =
    verb === 'stop'
      ? " Ses services seront indisponibles jusqu'à son prochain démarrage ; Docker le relance seulement si sa politique de redémarrage le prévoit."
      : verb === 'restart'
        ? ' Ses services seront brièvement interrompus.'
        : ''
  return `${verbTexts[verb].label} le conteneur ${name} ?${consequence}`
}

const failureMessages: Record<ContainerActionFailure, string> = {
  not_found: 'Docker ne connaît plus ce conteneur',
  rejected: "Docker a refusé ou n'a pas pu exécuter l'action",
  not_allowed:
    'le proxy Docker refuse les actions (MONOOPS_DOCKER_PROXY_CONTAINER_ACTIONS est désactivé)',
  timeout: "Docker n'a pas répondu en 90 secondes",
  unavailable: "le proxy Docker ou Docker n'est pas joignable",
  interrupted: 'le Manager a été arrêté pendant le suivi',
}

/** Where an action stands, as shown under the buttons. */
export function describeContainerAction(action: ContainerAction): {
  tone: BadgeTone
  message: string
} {
  const texts = verbTexts[action.action]
  if (action.state === 'running')
    return { tone: 'info', message: `${texts.progress} de ${action.name} en cours…` }
  if (action.state === 'succeeded')
    return { tone: 'success', message: `${action.name} ${texts.done}.` }
  const reason = action.failure ? failureMessages[action.failure] : 'cause inconnue'
  return { tone: 'danger', message: `${texts.progress} de ${action.name} en échec : ${reason}.` }
}

export type LogStreamFilter = 'all' | 'stdout' | 'stderr'

/** Number of last lines the logs card can ask for. */
export const logTailOptions = [100, 200, 500, 1000, 2000] as const

/** Lines of the chosen stream whose text holds the search, case ignored. */
export function filterLogLines(
  lines: ContainerLogLine[],
  stream: LogStreamFilter,
  search: string,
): ContainerLogLine[] {
  const needle = search.trim().toLocaleLowerCase('fr')
  return lines.filter(
    (line) =>
      (stream === 'all' || line.stream === stream) &&
      (needle === '' || line.message.toLocaleLowerCase('fr').includes(needle)),
  )
}
