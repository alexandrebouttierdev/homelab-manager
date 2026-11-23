import { requestJSON } from './client'

export type PowerAction = 'reboot' | 'poweroff'

export type PowerSettings = { enabled: boolean; actions: PowerAction[] }

export function getPowerSettings() {
  return requestJSON<PowerSettings>('/api/v1/machine/power')
}

export function requestPowerAction(action: PowerAction, confirmHostname: string) {
  return requestJSON<{ requested: boolean; action: PowerAction }>(
    `/api/v1/machine/power/${action}`,
    { method: 'POST', body: JSON.stringify({ confirmHostname }) },
  )
}
