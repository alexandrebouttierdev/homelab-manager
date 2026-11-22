/* eslint-disable react-refresh/only-export-components -- provider and hooks are intentionally colocated */
import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'

export const refreshChoices = [
  { value: 5_000, label: '5 s' },
  { value: 15_000, label: '15 s' },
  { value: 30_000, label: '30 s' },
  { value: 60_000, label: '60 s' },
  { value: 0, label: 'Pause' },
] as const

type RefreshContextValue = {
  interval: number
  setInterval: (value: number) => void
}

const RefreshContext = createContext<RefreshContextValue | null>(null)
const storageKey = 'homelab-manager-refresh'

function readInitialInterval(): number {
  if (typeof window === 'undefined') return 15_000
  const stored = window.localStorage.getItem(storageKey)
  if (stored === null) return 15_000
  const value = Number(stored)
  return refreshChoices.some((choice) => choice.value === value) ? value : 15_000
}

export function RefreshProvider({ children }: { children: ReactNode }) {
  const [interval, setIntervalState] = useState(readInitialInterval)

  useEffect(() => {
    window.localStorage.setItem(storageKey, String(interval))
  }, [interval])

  const value = useMemo(
    () => ({ interval, setInterval: setIntervalState }),
    [interval],
  )

  return <RefreshContext.Provider value={value}>{children}</RefreshContext.Provider>
}

export function useRefreshInterval(): number {
  const context = useContext(RefreshContext)
  if (!context) throw new Error('useRefreshInterval must be used within RefreshProvider')
  return context.interval
}

export function useRefreshControl(): RefreshContextValue {
  const context = useContext(RefreshContext)
  if (!context) throw new Error('useRefreshControl must be used within RefreshProvider')
  return context
}
