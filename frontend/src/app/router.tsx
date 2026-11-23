import { createBrowserRouter } from 'react-router-dom'
import { LoginPage } from '../features/auth/LoginPage'
import { ForcedPasswordChangePage } from '../features/auth/ForcedPasswordChangePage'
import { SecondFactorSetupPage } from '../features/auth/SecondFactorSetupPage'
import { DashboardPage } from '../features/dashboard/DashboardPage'
import { HostPage } from '../features/host/HostPage'
import { ContainersPage } from '../features/containers/ContainersPage'
import { ContainerDetailPage } from '../features/containers/ContainerDetailPage'
import { ImagesPage } from '../features/images/ImagesPage'
import { VolumesPage } from '../features/volumes/VolumesPage'
import { NetworksPage } from '../features/networks/NetworksPage'
import { StoragePage } from '../features/storage/StoragePage'
import { MetricsPage } from '../features/metrics/MetricsPage'
import { NetworkPage } from '../features/network/NetworkPage'
import { SettingsPage } from '../features/settings/SettingsPage'
import { EventsPage } from '../features/events/EventsPage'
import { JournalPage } from '../features/journal/JournalPage'
import { ServicesPage } from '../features/services/ServicesPage'
import { UsersPage } from '../features/users/UsersPage'
import { RequireAuth } from './RequireAuth'
import { AppLayout } from './AppLayout'

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
    children: [
      { path: '/', element: <DashboardPage /> },
      { path: '/host', element: <HostPage /> },
      { path: '/storage', element: <StoragePage /> },
      { path: '/network', element: <NetworkPage /> },
      { path: '/metrics', element: <MetricsPage /> },
      { path: '/containers', element: <ContainersPage /> },
      { path: '/containers/:containerID', element: <ContainerDetailPage /> },
      { path: '/images', element: <ImagesPage /> },
      { path: '/volumes', element: <VolumesPage /> },
      { path: '/networks', element: <NetworksPage /> },
      { path: '/events', element: <EventsPage /> },
      { path: '/logs', element: <JournalPage /> },
      { path: '/services', element: <ServicesPage /> },
      { path: '/settings', element: <SettingsPage /> },
      { path: '/users', element: <UsersPage /> },
    ],
  },
])
