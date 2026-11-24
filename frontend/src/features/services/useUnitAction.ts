import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { getLastUnitAction, requestUnitAction, type UnitAction } from '../../lib/api/services'
import {
  describeUnitAction,
  unitActionQuestion,
  type UnitActionChoice,
} from '../../lib/unitActions'

/** Inventory and unit details: read again once an action ends. */
const unitsQueryKey = ['system', 'units'] as const
const pollInterval = 1000

/**
 * Asks for confirmation, requests a unit action, then follows it until it
 * ends (SPEC_ADMINISTRATION.md § 5.2). Shared by the services page and the
 * alerts page.
 */
export function useUnitAction() {
  const queryClient = useQueryClient()
  const [pending, setPending] = useState<{ choice: UnitActionChoice; from?: string } | null>(null)
  const [followed, setFollowed] = useState<string | null>(null)
  // requestedAt of the action whose end already refreshed the inventory.
  const refreshedFor = useRef<string | null>(null)

  const followQuery = useQuery({
    queryKey: ['system', 'unit-actions', followed],
    queryFn: async (): Promise<UnitAction> => {
      const action = await getLastUnitAction(followed ?? '')
      if (action.state !== 'running' && refreshedFor.current !== action.requestedAt) {
        refreshedFor.current = action.requestedAt
        void queryClient.invalidateQueries({ queryKey: unitsQueryKey })
      }
      return action
    },
    enabled: followed !== null,
    retry: false,
    refetchInterval: (query) => (query.state.data?.state === 'running' ? pollInterval : false),
  })
  const mutation = useMutation({
    mutationFn: (choice: UnitActionChoice) => requestUnitAction(choice.unit, choice.verb),
    onSuccess: (action) => {
      queryClient.setQueryData(['system', 'unit-actions', action.unit], action)
      setFollowed(action.unit)
      setPending(null)
    },
  })

  const action = followed ? (followQuery.data ?? null) : null
  return {
    /** Asks for confirmation; from names the timer whose service runs. */
    choose: (choice: UnitActionChoice, from?: string) => {
      mutation.reset()
      setPending({ choice, from })
    },
    pending: pending
      ? {
          label: pending.choice.label,
          question: unitActionQuestion(pending.choice, pending.from),
          danger: pending.choice.verb === 'stop',
        }
      : null,
    confirm: () => {
      if (pending) mutation.mutate(pending.choice)
    },
    cancel: () => {
      mutation.reset()
      setPending(null)
    },
    isSubmitting: mutation.isPending,
    errorMessage: mutation.error?.message ?? (followed ? followQuery.error?.message : null) ?? null,
    /** An action in progress blocks new requests from the same panel. */
    busy: mutation.isPending || action?.state === 'running',
    progress: action ? describeUnitAction(action) : null,
    dismiss: () => setFollowed(null),
  }
}

export type UnitActionModel = ReturnType<typeof useUnitAction>
