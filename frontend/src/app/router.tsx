import { createBrowserRouter } from 'react-router-dom'
import { LoginPage } from '../features/auth/LoginPage'
import { DashboardPage } from '../features/dashboard/DashboardPage'
import { HostPage } from '../features/host/HostPage'
import { ContainersPage } from '../features/containers/ContainersPage'
import { ContainerDetailPage } from '../features/containers/ContainerDetailPage'
import { MetricsPage } from '../features/metrics/MetricsPage'
import { NetworkPage } from '../features/network/NetworkPage'
import { SettingsPage } from '../features/settings/SettingsPage'
import { EventsPage } from '../features/events/EventsPage'
import { RequireAuth } from './RequireAuth'
import { AppLayout } from './AppLayout'

export const router = createBrowserRouter([
  { path: '/login', element: <LoginPage /> },
  {
    element: <RequireAuth><AppLayout /></RequireAuth>,
    children: [
      { path: '/', element: <DashboardPage /> },
      { path: '/host', element: <HostPage /> },
      { path: '/containers', element: <ContainersPage /> },
      { path: '/containers/:containerID', element: <ContainerDetailPage /> },
      { path: '/metrics', element: <MetricsPage /> },
      { path: '/network', element: <NetworkPage /> },
      { path: '/settings', element: <SettingsPage /> },
      { path: '/events', element: <EventsPage /> },
    ],
  },
])
