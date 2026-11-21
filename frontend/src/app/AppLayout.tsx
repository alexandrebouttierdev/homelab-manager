import { useMutation, useQueryClient } from '@tanstack/react-query'
import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { logout } from '../lib/api/auth'

const navigation = [
  { to: '/', label: "Vue d'ensemble", end: true },
  { to: '/host', label: 'Machine' },
  { to: '/containers', label: 'Conteneurs' },
  { to: '/metrics', label: 'Métriques' },
  { to: '/network', label: 'Réseau' },
  { to: '/events', label: 'Événements' },
  { to: '/settings', label: 'Paramètres' },
]

export function AppLayout() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const signOut = useMutation({
    mutationFn: logout,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['session'] })
      navigate('/login', { replace: true })
    },
  })

  return (
    <main className="dashboard-shell">
      <aside className="sidebar">
        <strong>Homelab Manager</strong>
        <nav aria-label="Navigation principale">
          {navigation.map(({ to, label, end }) => (
            <NavLink key={to} to={to} end={end}>{label}</NavLink>
          ))}
        </nav>
        <button type="button" className="logout-button" onClick={() => signOut.mutate()} disabled={signOut.isPending}>
          {signOut.isPending ? 'Déconnexion...' : 'Se déconnecter'}
        </button>
        {signOut.isError && <p className="field-error" role="alert">{signOut.error.message}</p>}
      </aside>
      <Outlet />
    </main>
  )
}
