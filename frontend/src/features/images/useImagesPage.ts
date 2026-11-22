import { useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { listContainers } from '../../lib/api/monitoring'
import { imageTag, isRunning } from '../../lib/containers'
import { useRefreshInterval } from '../../lib/refresh'

export type ImageGroup = {
  image: string
  repository: string
  tag: string
  containers: Array<{ id: string; name: string; state: string }>
  running: number
  /** Distinct states of the containers using this image. */
  states: string[]
}

/** Logic and view model of ImagesPage. */
export function useImagesPage() {
  const refreshInterval = useRefreshInterval()
  const [search, setSearch] = useState('')
  const containers = useQuery({
    queryKey: ['containers', false],
    queryFn: () => listContainers(false),
    refetchInterval: refreshInterval || false,
  })

  const images = useMemo(() => {
    const grouped = new Map<string, ImageGroup>()
    for (const container of containers.data ?? []) {
      const existing = grouped.get(container.image) ?? {
        image: container.image,
        ...imageTag(container.image),
        containers: [],
        running: 0,
        states: [],
      }
      existing.containers.push({ id: container.id, name: container.name, state: container.state })
      if (isRunning(container.state)) existing.running += 1
      if (!existing.states.includes(container.state)) existing.states.push(container.state)
      grouped.set(container.image, existing)
    }
    return Array.from(grouped.values()).sort((left, right) => left.image.localeCompare(right.image))
  }, [containers.data])

  const filtered = images.filter(
    (image) =>
      !search ||
      `${image.image} ${image.repository} ${image.tag}`
        .toLowerCase()
        .includes(search.toLowerCase()),
  )

  return {
    search,
    setSearch,
    isLoading: containers.isLoading,
    error: containers.error,
    retry: () => void containers.refetch(),
    containerCount: containers.data?.length ?? 0,
    activeImageCount: images.filter((image) => image.running > 0).length,
    images,
    filtered,
  }
}
