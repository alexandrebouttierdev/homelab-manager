import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useRef, useState } from 'react'
import { useSession } from '../../hooks/useSession'
import { loadState } from '../../hooks/loadState'
import {
  getContainerActionSettings,
  getContainerLogs,
  getLastContainerAction,
  requestContainerAction,
  type ContainerAction,
  type ContainerVerb,
} from '../../lib/api/containers'
import {
  containerActionChoices,
  containerActionQuestion,
  containerVerbLabel,
  describeContainerAction,
  filterLogLines,
  type LogStreamFilter,
} from '../../lib/containerActions'
import { canManageContainers } from '../../lib/roles'

const pollInterval = 1000

/** End of the log of a container, its filters and its refresh (D-5). */
export function useContainerLogs(containerID: string, removed: boolean) {
  const [tail, setTail] = useState(200)
  const [stream, setStream] = useState<LogStreamFilter>('all')
  const [search, setSearch] = useState('')
  const [follow, setFollow] = useState(false)
  const logs = useQuery({
    queryKey: ['container', containerID, 'logs', tail],
    queryFn: () => getContainerLogs(containerID, tail),
    enabled: Boolean(containerID) && !removed,
    retry: false,
    refetchInterval: follow ? 5000 : false,
  })
  const lines = useMemo(
    () => filterLogLines(logs.data?.lines ?? [], stream, search),
    [logs.data, stream, search],
  )
  return {
    state: loadState(logs),
    removed,
    tail,
    setTail,
    stream,
    setStream,
    search,
    setSearch,
    follow,
    setFollow,
    lines,
    total: logs.data?.lines.length ?? 0,
    truncated: logs.data?.truncated ?? false,
    isFetching: logs.isFetching,
  }
}

export type ContainerLogsModel = ReturnType<typeof useContainerLogs>

/**
 * Offers the actions that fit the state of a container, asks for
 * confirmation, requests the action, then follows it until it ends (D-5).
 */
export function useContainerAction(
  containerID: string,
  name: string,
  state: string,
  removed: boolean,
) {
  const queryClient = useQueryClient()
  const { user } = useSession()
  const [pending, setPending] = useState<ContainerVerb | null>(null)
  const [followed, setFollowed] = useState(false)
  const refreshedFor = useRef<string | null>(null)
  const allowed = canManageContainers(user?.role)

  const settings = useQuery({
    queryKey: ['docker', 'container-actions'],
    queryFn: getContainerActionSettings,
    enabled: allowed,
    staleTime: 60_000,
  })
  const followQuery = useQuery({
    queryKey: ['container', containerID, 'action'],
    queryFn: async (): Promise<ContainerAction> => {
      const action = await getLastContainerAction(containerID)
      if (action.state !== 'running' && refreshedFor.current !== action.requestedAt) {
        refreshedFor.current = action.requestedAt
        void queryClient.invalidateQueries({ queryKey: ['container', containerID] })
        void queryClient.invalidateQueries({ queryKey: ['containers'] })
      }
      return action
    },
    enabled: followed,
    retry: false,
    refetchInterval: (query) => (query.state.data?.state === 'running' ? pollInterval : false),
  })
  const mutation = useMutation({
    mutationFn: (verb: ContainerVerb) => requestContainerAction(containerID, verb),
    onSuccess: (action) => {
      queryClient.setQueryData(['container', containerID, 'action'], action)
      setFollowed(true)
      setPending(null)
    },
  })

  const enabled = settings.data?.enabled ?? false
  const action = followed ? (followQuery.data ?? null) : null
  const choices = allowed
    ? containerActionChoices(state, removed, enabled).map((verb) => ({
        verb,
        label: containerVerbLabel(verb),
        danger: verb === 'stop',
      }))
    : []
  return {
    visible: allowed,
    /** Actions are off in the Manager: say so to those who could use them. */
    disabledNotice: allowed && settings.data !== undefined && !enabled,
    choices,
    choose: (verb: ContainerVerb) => {
      mutation.reset()
      setPending(verb)
    },
    pending: pending
      ? {
          label: containerVerbLabel(pending),
          question: containerActionQuestion(pending, name),
          danger: pending === 'stop',
        }
      : null,
    confirm: () => {
      if (pending) mutation.mutate(pending)
    },
    cancel: () => {
      mutation.reset()
      setPending(null)
    },
    isSubmitting: mutation.isPending,
    errorMessage: mutation.error?.message ?? (followed ? followQuery.error?.message : null) ?? null,
    busy: mutation.isPending || action?.state === 'running',
    progress: action ? describeContainerAction(action) : null,
    dismiss: () => setFollowed(false),
  }
}

export type ContainerActionModel = ReturnType<typeof useContainerAction>
