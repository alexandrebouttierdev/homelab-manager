import type { ComponentType } from 'react'
import { createBrowserRouter } from 'react-router-dom'
import { LoginPage } from '../features/auth/LoginPage'
import { ForcedPasswordChangePage } from '../features/auth/ForcedPasswordChangePage'
import { SecondFactorSetupPage } from '../features/auth/SecondFactorSetupPage'
import { RequireAuth } from './RequireAuth'
import { AppLayout } from './AppLayout'

// Each page is its own chunk, loaded on first visit: the login page and the
// layout stay in the main bundle, the pages and their libraries do not.
function page<M>(load: () => Promise<M>, pick: (module: M) => ComponentType) {
  return async () => ({ Component: pick(await load()) })
}

export const router = createBrowserRouter([
  { path: '/login', element: <LoginPage /> },
  { path: '/password-change', element: <ForcedPasswordChangePage /> },
  { path: '/second-factor-setup', element: <SecondFactorSetupPage /> },
  {
    element: (
      <RequireAuth>
        <AppLayout />
      </RequireAuth>
    ),
    // Shown while the first page of the session is loaded.
    hydrateFallbackElement: <div className="min-h-screen bg-background" aria-busy="true" />,
    children: [
      {
        path: '/',
        lazy: page(
          () => import('../features/dashboard/DashboardPage'),
          (m) => m.DashboardPage,
        ),
      },
      {
        path: '/host',
        lazy: page(
          () => import('../features/host/HostPage'),
          (m) => m.HostPage,
        ),
      },
      {
        path: '/storage',
        lazy: page(
          () => import('../features/storage/StoragePage'),
          (m) => m.StoragePage,
        ),
      },
      {
        path: '/network',
        lazy: page(
          () => import('../features/network/NetworkPage'),
          (m) => m.NetworkPage,
        ),
      },
      {
        path: '/metrics',
        lazy: page(
          () => import('../features/metrics/MetricsPage'),
          (m) => m.MetricsPage,
        ),
      },
      {
        path: '/containers',
        lazy: page(
          () => import('../features/containers/ContainersPage'),
          (m) => m.ContainersPage,
        ),
      },
      {
        path: '/containers/:containerID',
        lazy: page(
          () => import('../features/containers/ContainerDetailPage'),
          (m) => m.ContainerDetailPage,
        ),
      },
      {
        path: '/images',
        lazy: page(
          () => import('../features/images/ImagesPage'),
          (m) => m.ImagesPage,
        ),
      },
      {
        path: '/volumes',
        lazy: page(
          () => import('../features/volumes/VolumesPage'),
          (m) => m.VolumesPage,
        ),
      },
      {
        path: '/networks',
        lazy: page(
          () => import('../features/networks/NetworksPage'),
          (m) => m.NetworksPage,
        ),
      },
      {
        path: '/alerts',
        lazy: page(
          () => import('../features/alerts/AlertsPage'),
          (m) => m.AlertsPage,
        ),
      },
      {
        path: '/events',
        lazy: page(
          () => import('../features/events/EventsPage'),
          (m) => m.EventsPage,
        ),
      },
      {
        path: '/logs',
        lazy: page(
          () => import('../features/journal/JournalPage'),
          (m) => m.JournalPage,
        ),
      },
      {
        path: '/services',
        lazy: page(
          () => import('../features/services/ServicesPage'),
          (m) => m.ServicesPage,
        ),
      },
      {
        path: '/backups',
        lazy: page(
          () => import('../features/backups/BackupsPage'),
          (m) => m.BackupsPage,
        ),
      },
      {
        path: '/cron',
        lazy: page(
          () => import('../features/cron/CronPage'),
          (m) => m.CronPage,
        ),
      },
      {
        path: '/settings',
        lazy: page(
          () => import('../features/settings/SettingsPage'),
          (m) => m.SettingsPage,
        ),
      },
      {
        path: '/users',
        lazy: page(
          () => import('../features/users/UsersPage'),
          (m) => m.UsersPage,
        ),
      },
    ],
  },
])
