/* eslint-disable react-refresh/only-export-components -- provider and hooks are intentionally colocated */
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'

export type Theme = 'dark' | 'light'

type ThemeContextValue = {
  theme: Theme
  setTheme: (theme: Theme) => void
  toggleTheme: () => void
}

const ThemeContext = createContext<ThemeContextValue | null>(null)
const storageKey = 'homelab-manager-theme'

function readInitialTheme(): Theme {
  if (typeof window === 'undefined') return 'dark'
  const stored = window.localStorage.getItem(storageKey)
  if (stored === 'light' || stored === 'dark') return stored
  if (
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-color-scheme: light)').matches
  ) {
    return 'light'
  }
  return 'dark'
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(readInitialTheme)

  useEffect(() => {
    document.documentElement.dataset.theme = theme
    window.localStorage.setItem(storageKey, theme)
  }, [theme])

  const setTheme = useCallback((nextTheme: Theme) => {
    setThemeState(nextTheme)
  }, [])

  const toggleTheme = useCallback(() => {
    setThemeState((current) => (current === 'dark' ? 'light' : 'dark'))
  }, [])

  const value = useMemo(() => ({ theme, setTheme, toggleTheme }), [setTheme, theme, toggleTheme])

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

export function useTheme(): ThemeContextValue {
  const context = useContext(ThemeContext)
  if (!context) throw new Error('useTheme must be used within ThemeProvider')
  return context
}

export function useChartTheme() {
  const { theme } = useTheme()
  return useMemo(
    () =>
      theme === 'light'
        ? {
            text: '#4b5563',
            muted: '#6b7280',
            grid: 'rgba(20, 24, 31, 0.08)',
            axis: '#c8ced8',
            tooltipBackground: '#ffffff',
            tooltipBorder: '#dde1e7',
            tooltipText: '#14181f',
            cpu: '#2563eb',
            ram: '#7c3aed',
            net: '#059669',
            accent: '#4f5fd7',
          }
        : {
            text: '#9299a3',
            muted: '#686f79',
            grid: 'rgba(232, 234, 237, 0.06)',
            axis: '#30343a',
            tooltipBackground: '#22262b',
            tooltipBorder: '#30343a',
            tooltipText: '#e8eaed',
            cpu: '#6d9fe8',
            ram: '#a78bfa',
            net: '#45b881',
            accent: '#8b9cfb',
          },
    [theme],
  )
}
