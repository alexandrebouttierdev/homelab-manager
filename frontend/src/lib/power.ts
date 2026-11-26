import type { PowerAction } from './api/machine'

export const powerActionList: PowerAction[] = ['reboot', 'poweroff']

export const powerActionTexts: Record<
  PowerAction,
  { label: string; warning: string; done: string }
> = {
  reboot: {
    label: 'Redémarrer',
    warning:
      "Tous les services et conteneurs seront arrêtés puis relancés. L'interface sera indisponible pendant le redémarrage.",
    done: "Redémarrage demandé. L'interface reviendra quand la machine aura redémarré.",
  },
  poweroff: {
    label: 'Éteindre',
    warning:
      "La machine s'arrêtera complètement. MonoOps ne pourra pas la rallumer : il faudra un accès physique ou Wake-on-LAN.",
    done: "Extinction demandée. La machine va s'arrêter.",
  },
}
