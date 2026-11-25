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
            text: '#4a4a44',
            muted: '#6b6b64',
            grid: 'rgba(23, 23, 21, 0.07)',
            axis: '#d2d2ca',
            tooltipBackground: '#ffffff',
            tooltipBorder: '#e4e4de',
            tooltipText: '#171715',
            cpu: '#2458c6',
            ram: '#c2610f',
            net: '#0f766e',
            accent: '#171715',
            danger: '#c8372d',
            warning: '#9a6500',
          }
        : {
            text: '#b4b4ab',
            muted: '#8c8c84',
            grid: 'rgba(237, 237, 230, 0.06)',
            axis: '#34342f',
            tooltipBackground: '#1d1d1b',
            tooltipBorder: '#34342f',
            tooltipText: '#edede6',
            cpu: '#78a9ff',
            ram: '#f2a65a',
            net: '#7cc4c0',
            accent: '#edede6',
            danger: '#f26b5e',
            warning: '#e8b85a',
          },
    [theme],
  )
}
