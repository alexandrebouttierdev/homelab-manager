import { useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { loadState } from '../../hooks/loadState'
import { getUnit, listUnits } from '../../lib/api/services'
import { useRefreshInterval } from '../../lib/refresh'
import {
  filterUnits,
  isUnitTypeFilter,
  oldestFirst,
  unitStateCounts,
  unitsStatusInfo,
  unitTypeOptions,
  type UnitStateFilter,
  type UnitTypeFilter,
} from '../../lib/services'

/** Logic and view model of ServicesPage. The selected unit lives in the URL (?unit=). */
export function useServicesPage() {
  const refreshInterval = useRefreshInterval()
  const [type, setType] = useState<UnitTypeFilter>('all')
  const [state, setState] = useState<UnitStateFilter>('all')
  const [search, setSearch] = useState('')
  const [params, setParams] = useSearchParams()
  const selectedName = params.get('unit')

  const inventoryQuery = useQuery({
    queryKey: ['system', 'units'],
    queryFn: listUnits,
    refetchInterval: refreshInterval || false,
  })
  const detailQuery = useQuery({
    queryKey: ['system', 'units', selectedName],
    queryFn: () => getUnit(selectedName ?? ''),
    enabled: selectedName !== null,
    retry: false,
  })

  const allUnits = useMemo(() => inventoryQuery.data?.units ?? [], [inventoryQuery.data])
  const units = useMemo(
    () => filterUnits(allUnits, { type, state, search }),
    [allUnits, type, state, search],
  )
  const stateCounts = unitStateCounts(allUnits)
  const inventory = inventoryQuery.data
  const journal = useMemo(
    () => oldestFirst(detailQuery.data?.journal.entries ?? []),
    [detailQuery.data],
  )

  const select = (name: string | null) => {
    setParams(name ? { unit: name } : {}, { replace: true })
  }

  return {
    inventory: {
      state: loadState(inventoryQuery),
      loaded: inventory !== undefined,
      status: inventory ? unitsStatusInfo(inventory.status) : null,
      unavailable: inventory?.status === 'unavailable',
      // An error keeps the last inventory: it is shown as possibly outdated.
      outdated: inventory?.status === 'error' && inventory.collectedAt !== null,
      collectedAt: inventory?.collectedAt ?? null,
      // Without a successful collection the counters are unknown, not 0.
      counts: inventory?.collectedAt ? inventory.counts : null,
      total: allUnits.length,
      refresh: () => void inventoryQuery.refetch(),
    },
    filters: {
      type,
      setType: (value: string) => {
        if (isUnitTypeFilter(value)) setType(value)
      },
      typeOptions: unitTypeOptions,
      state,
      setState,
      stateOptions: [
        { value: 'all' as const, label: 'Tous', count: stateCounts.all },
        { value: 'failed' as const, label: 'En échec', count: stateCounts.failed },
        { value: 'active' as const, label: 'Actifs', count: stateCounts.active },
        { value: 'other' as const, label: 'Autres', count: stateCounts.other },
      ],
      search,
      setSearch,
      reset: () => {
        setType('all')
        setState('all')
        setSearch('')
      },
    },
    units,
    selectedName,
    select,
    detail: {
      state: loadState(detailQuery),
      data: detailQuery.data ?? null,
      journal,
    },
  }
}

export type ServicesPageModel = ReturnType<typeof useServicesPage>
