export type ContainerStateTone = 'success' | 'warning' | 'danger' | 'neutral' | 'accent'

export function containerStateTone(state: string | null | undefined): ContainerStateTone {
  switch (state) {
    case 'running':
      return 'success'
    case 'restarting':
    case 'paused':
      return 'warning'
    case 'dead':
      return 'danger'
    case 'exited':
    case 'created':
      return 'neutral'
    default:
      return 'neutral'
  }
}

export function containerStateLabel(state: string | null | undefined): string {
  switch (state) {
    case 'running':
      return 'En ligne'
    case 'exited':
      return 'Arrêté'
    case 'created':
      return 'Créé'
    case 'restarting':
      return 'Redémarrage'
    case 'paused':
      return 'En pause'
    case 'dead':
      return 'Hors service'
    case 'removing':
      return 'Suppression'
    default:
      return state || 'Inconnu'
  }
}

export function isRunning(state: string | null | undefined): boolean {
  return state === 'running'
}

export function isStopped(state: string | null | undefined): boolean {
  return state === 'exited' || state === 'created'
}

export function isErrorState(state: string | null | undefined): boolean {
  return state === 'restarting' || state === 'dead' || state === 'paused'
}

type RecordValue = Record<string, unknown>

function field(record: RecordValue, ...keys: string[]): unknown {
  for (const key of keys) {
    if (record[key] !== undefined && record[key] !== null) return record[key]
  }
  return undefined
}

export function formatPorts(ports: Array<Record<string, unknown>>): string[] {
  return ports.map((port) => {
    const privatePort = field(port, 'privatePort', 'PrivatePort')
    const publicPort = field(port, 'publicPort', 'PublicPort')
    const type = field(port, 'type', 'Type') ?? 'tcp'
    const ip = field(port, 'ip', 'IP')
    const target = privatePort ? `${String(privatePort)}/${String(type)}` : String(type)
    if (!publicPort) return target
    const host = ip ? `${String(ip)}:${String(publicPort)}` : String(publicPort)
    return `${host} → ${target}`
  })
}

export function mountField(mount: Record<string, unknown>, key: 'type' | 'source' | 'destination' | 'rw'): string {
  const value = field(mount, key, key.charAt(0).toUpperCase() + key.slice(1))
  if (typeof value === 'boolean') return value ? 'Lecture/écriture' : 'Lecture seule'
  return value == null ? '' : String(value)
}

export function shortId(value: string | null | undefined, length = 12): string {
  if (!value) return '—'
  return value.length > length ? value.slice(0, length) : value
}

export function imageTag(image: string): { repository: string; tag: string } {
  const withoutDigest = image.split('@')[0]
  const separator = withoutDigest.lastIndexOf(':')
  const slash = withoutDigest.lastIndexOf('/')
  if (separator > slash) {
    return { repository: withoutDigest.slice(0, separator), tag: withoutDigest.slice(separator + 1) }
  }
  return { repository: withoutDigest, tag: 'latest' }
}
