import { useState } from 'react'
import { Outlet } from 'react-router-dom'
import { AppSidebar } from '../components/layout/AppSidebar'
import { TopBar } from '../components/layout/TopBar'
import { useScrollToTopOnNavigation } from '../hooks/useScrollToTopOnNavigation'

export function AppLayout() {
  const [sidebarOpen, setSidebarOpen] = useState(false)
  useScrollToTopOnNavigation()

  return (
    <div className="min-h-screen bg-background">
      <a
        href="#contenu"
        className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-[60] focus:rounded-md focus:bg-accent focus:px-3 focus:py-2 focus:text-sm focus:font-medium focus:text-on-accent"
      >
        Aller au contenu
      </a>
      <AppSidebar open={sidebarOpen} onClose={() => setSidebarOpen(false)} />
      <div className="min-h-screen lg:pl-60">
        <TopBar onOpenMenu={() => setSidebarOpen(true)} />
        <main
          id="contenu"
          className="mx-auto w-full max-w-[1440px] px-4 py-6 sm:px-6 lg:px-8 lg:py-8"
        >
          <Outlet />
        </main>
      </div>
    </div>
  )
}
