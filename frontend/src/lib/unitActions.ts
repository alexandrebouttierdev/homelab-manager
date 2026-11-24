import type { BadgeTone } from '../components/ui/Badge'
import type { SystemUnit, UnitAction, UnitActionFailure, UnitVerb } from './api/services'

/** An action a button offers: the verb, and the unit it applies to (a timer runs its service). */
export type UnitActionChoice = { unit: string; verb: UnitVerb; label: string }

const verbTexts: Record<UnitVerb, { label: string; progress: string; done: string }> = {
  start: { label: 'Démarrer', progress: 'Démarrage', done: 'démarrée' },
  stop: { label: 'Arrêter', progress: 'Arrêt', done: 'arrêtée' },
  restart: { label: 'Redémarrer', progress: 'Redémarrage', done: 'redémarrée' },
  reload: { label: 'Recharger', progress: 'Rechargement', done: 'rechargée' },
}

export function unitVerbLabel(verb: UnitVerb): string {
  return verbTexts[verb].label
}

const runningStates = new Set(['active', 'reloading', 'activating'])

/**
 * Actions offered on a unit (SPEC_ADMINISTRATION.md § 5.2): the verbs that
 * fit its state when it is in the allow list, and « Exécuter » on a timer
 * whose service is in the allow list. The server checks again.
 */
export function unitActionChoices(unit: SystemUnit, manageable: string[]): UnitActionChoice[] {
  const allowed = new Set(manageable)
  const choices: UnitActionChoice[] = []
  if (unit.type === 'timer') {
    const service = unit.triggers.find((name) => allowed.has(name))
    if (service) choices.push({ unit: service, verb: 'start', label: 'Exécuter' })
  }
  if (allowed.has(unit.name)) {
    const verbs: UnitVerb[] = runningStates.has(unit.activeState)
      ? ['restart', 'reload', 'stop']
      : ['start', 'restart']
    for (const verb of verbs) choices.push({ unit: unit.name, verb, label: verbTexts[verb].label })
  }
  return choices
}

/** The question asked before an action: it names the target. */
export function unitActionQuestion(choice: UnitActionChoice, from?: string): string {
  if (from && from !== choice.unit)
    return `Exécuter maintenant ${choice.unit}, déclenché par ${from} ?`
  const consequence =
    choice.verb === 'stop'
      ? " Le service sera indisponible jusqu'à son prochain démarrage."
      : choice.verb === 'restart'
        ? ' Le service sera brièvement interrompu.'
        : ''
  return `${verbTexts[choice.verb].label} ${choice.unit} ?${consequence}`
}

const failureMessages: Record<UnitActionFailure, string> = {
  failed: "l'unité est en échec : consultez son journal",
  not_allowed:
    "polkit a refusé : l'unité manque dans la règle installée (régénérez-la avec « homelab-manager polkit-rule »)",
  not_found: 'systemd ne connaît pas cette unité',
  rejected: "systemd a refusé l'action (non applicable à cette unité, unité masquée…)",
  timeout: "l'unité n'a pas atteint un état stable en 90 secondes",
  unavailable: "systemd n'est pas joignable sur le bus système",
  interrupted: 'le Manager a été arrêté pendant le suivi',
}

/** Where an action stands, as shown under the buttons. */
export function describeUnitAction(action: UnitAction): { tone: BadgeTone; message: string } {
  const texts = verbTexts[action.action]
  if (action.state === 'running')
    return { tone: 'info', message: `${texts.progress} de ${action.unit} en cours…` }
  const final = action.final ? ` (état : ${action.final.activeState})` : ''
  if (action.state === 'succeeded')
    return { tone: 'success', message: `${action.unit} ${texts.done}${final}.` }
  const reason = action.failure ? failureMessages[action.failure] : 'cause inconnue'
  return {
    tone: 'danger',
    message: `${texts.progress} de ${action.unit} en échec : ${reason}${final}.`,
  }
}
