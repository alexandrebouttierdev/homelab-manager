import { useEffect } from 'react'
import { useLocation } from 'react-router-dom'

/** Scrolls back to the top when the route changes. */
export function useScrollToTopOnNavigation() {
  const { pathname } = useLocation()
  useEffect(() => {
    window.scrollTo({ top: 0 })
  }, [pathname])
}
