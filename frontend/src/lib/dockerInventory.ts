import type { DockerInventory } from './api/docker'
import { formatBytes } from './format'

/** One image, volume or network that no container uses. */
export type UnusedItem = {
  key: string
  name: string
  detail: string
  createdAt: string | null
}

/** View model of an "unused" section of the Docker pages. */
export type UnusedSection = {
  /** null while the inventory is not read yet. */
  items: UnusedItem[] | null
  /** Why the list may be missing or old, in French; null when current. */
  notice: string | null
  /** Total size of the items, for images only. */
  totalBytes: number | null
}

function shortId(id: string): string {
  return id.replace(/^sha256:/, '').slice(0, 12)
}

/** Explains a missing, partial or old inventory. */
export function inventoryNotice(inventory: DockerInventory | undefined): string | null {
  if (!inventory) return null
  if (inventory.status === 'starting')
    return 'Inventaire Docker en cours de lecture : il est relu chaque minute.'
  if (inventory.status === 'unavailable')
    return inventory.observedAt
      ? 'Docker est injoignable : la liste date de la dernière lecture réussie.'
      : 'Docker est injoignable ou le proxy Docker n’autorise pas encore ces listes.'
  if (inventory.status === 'stale')
    return 'La dernière lecture de Docker date de plus de 3 minutes.'
  if (inventory.truncated)
    return 'Liste tronquée : Docker en renvoie plus que la limite du Manager.'
  return null
}

function section(
  inventory: DockerInventory | undefined,
  items: (inventory: DockerInventory) => UnusedItem[],
): UnusedSection {
  const notice = inventoryNotice(inventory)
  if (!inventory || !inventory.observedAt) return { items: null, notice, totalBytes: null }
  return { items: items(inventory), notice, totalBytes: null }
}

export function unusedImages(inventory: DockerInventory | undefined): UnusedSection {
  const result = section(inventory, (current) =>
    current.images
      .filter((image) => image.containers === 0)
      .map((image) => ({
        key: image.id,
        name: image.tags.length > 0 ? image.tags.join(', ') : `${shortId(image.id)} (sans tag)`,
        detail: formatBytes(image.sizeBytes),
        createdAt: image.createdAt,
      })),
  )
  if (inventory && result.items) {
    result.totalBytes = inventory.images
      .filter((image) => image.containers === 0)
      .reduce((total, image) => total + image.sizeBytes, 0)
  }
  return result
}

export function unusedVolumes(inventory: DockerInventory | undefined): UnusedSection {
  return section(inventory, (current) =>
    current.volumes
      .filter((volume) => volume.containers === 0)
      .map((volume) => ({
        key: volume.name,
        name: volume.name,
        detail: `Pilote ${volume.driver || '—'}`,
        createdAt: volume.createdAt,
      })),
  )
}

export function unusedNetworks(inventory: DockerInventory | undefined): UnusedSection {
  return section(inventory, (current) =>
    current.networks
      .filter((network) => network.containers === 0 && !network.predefined)
      .map((network) => ({
        key: network.id,
        name: network.name,
        detail: [
          `Pilote ${network.driver || '—'}`,
          ...network.subnets,
          ...(network.internal ? ['interne'] : []),
        ].join(' · '),
        createdAt: null,
      })),
  )
}

/** The section shown when the inventory cannot be read at all. */
export function unreadableSection(error: Error): UnusedSection {
  return { items: null, notice: `Inventaire Docker illisible : ${error.message}`, totalBytes: null }
}
