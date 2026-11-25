import { describe, expect, it } from 'vitest'
import type { DockerInventory } from './api/docker'
import {
  inventoryNotice,
  unreadableSection,
  unusedImages,
  unusedNetworks,
  unusedVolumes,
} from './dockerInventory'

function inventory(overrides: Partial<DockerInventory> = {}): DockerInventory {
  return {
    images: [
      {
        id: 'sha256:aaa',
        tags: ['nginx:stable'],
        createdAt: '2026-11-20T10:00:00Z',
        sizeBytes: 1024,
        containers: 2,
      },
      {
        id: 'sha256:0123456789abcdef',
        tags: [],
        createdAt: '2026-11-19T10:00:00Z',
        sizeBytes: 2048,
        containers: 0,
      },
      {
        id: 'sha256:bbb',
        tags: ['redis:7', 'redis:latest'],
        createdAt: '2026-11-18T10:00:00Z',
        sizeBytes: 1024,
        containers: 0,
      },
    ],
    volumes: [
      { name: 'web-data', driver: 'local', scope: 'local', createdAt: null, containers: 1 },
      {
        name: 'orphan',
        driver: 'local',
        scope: 'local',
        createdAt: '2026-11-20T10:00:00Z',
        containers: 0,
      },
    ],
    networks: [
      {
        id: 'n1',
        name: 'none',
        driver: 'null',
        scope: 'local',
        internal: false,
        predefined: true,
        subnets: [],
        containers: 0,
      },
      {
        id: 'n2',
        name: 'old',
        driver: 'bridge',
        scope: 'local',
        internal: true,
        predefined: false,
        subnets: ['172.30.0.0/16'],
        containers: 0,
      },
      {
        id: 'n3',
        name: 'front',
        driver: 'bridge',
        scope: 'local',
        internal: false,
        predefined: false,
        subnets: [],
        containers: 3,
      },
    ],
    truncated: false,
    observedAt: '2026-11-25T09:00:00Z',
    status: 'ok',
    lastSuccessAt: '2026-11-25T09:00:00Z',
    ...overrides,
  }
}

describe('unused Docker items', () => {
  it('keeps the images without container and adds up their size', () => {
    const section = unusedImages(inventory())
    expect(section.items?.map((item) => item.name)).toEqual([
      '0123456789ab (sans tag)',
      'redis:7, redis:latest',
    ])
    expect(section.totalBytes).toBe(3072)
    expect(section.notice).toBeNull()
  })

  it('keeps the volumes without container', () => {
    expect(unusedVolumes(inventory()).items?.map((item) => item.key)).toEqual(['orphan'])
  })

  it('leaves out the networks of Docker itself', () => {
    const items = unusedNetworks(inventory()).items
    expect(items?.map((item) => item.name)).toEqual(['old'])
    expect(items?.[0].detail).toBe('Pilote bridge · 172.30.0.0/16 · interne')
  })

  it('has no list before the first reading', () => {
    const section = unusedImages(inventory({ status: 'starting', observedAt: null, images: [] }))
    expect(section.items).toBeNull()
    expect(section.notice).toMatch(/en cours de lecture/)
    expect(unusedVolumes(undefined)).toEqual({ items: null, notice: null, totalBytes: null })
  })

  it('explains an unavailable, old or truncated inventory', () => {
    expect(inventoryNotice(inventory({ status: 'unavailable', observedAt: null }))).toMatch(
      /proxy Docker/,
    )
    expect(inventoryNotice(inventory({ status: 'unavailable' }))).toMatch(/dernière lecture/)
    expect(inventoryNotice(inventory({ status: 'stale' }))).toMatch(/3 minutes/)
    expect(inventoryNotice(inventory({ truncated: true }))).toMatch(/tronquée/)
    expect(unreadableSection(new Error('boom')).notice).toBe('Inventaire Docker illisible : boom')
  })
})
