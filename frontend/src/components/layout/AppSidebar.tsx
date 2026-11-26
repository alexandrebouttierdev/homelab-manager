import {
  Activity,
  Bell,
  Cable,
  CalendarClock,
  Container,
  Database,
  DatabaseBackup,
  Gauge,
  HardDrive,
  Images,
  LayoutDashboard,
  ScrollText,
  Server,
  Settings,
  Users,
  Wrench,
  X,
  type LucideIcon,
} from 'lucide-react'
import { NavLink } from 'react-router-dom'
import { useHost } from '../../hooks/useHost'
import { useSession } from '../../hooks/useSession'
import { cn } from '../../lib/cn'
import { formatUptime } from '../../lib/format'
import { Logo } from '../ui/Logo'
import { StatusDot } from '../ui/StatusDot'
import { APP_VERSION } from '../../lib/version'

type NavItem = {
  label: string
  to: string
  icon: LucideIcon
  end?: boolean
}

const dockerItems: NavItem[] = [
  { label: 'Tous les conteneurs', to: '/containers', icon: Container },
  { label: 'Images', to: '/images', icon: Images },
  { label: 'Volumes', to: '/volumes', icon: Database },
  { label: 'Réseaux Docker', to: '/networks', icon: Cable },
]

const systemItems: NavItem[] = [
  { label: 'Machine', to: '/host', icon: Server },
  { label: 'Stockage', to: '/storage', icon: HardDrive },
  { label: 'Réseau', to: '/network', icon: Cable },
  { label: 'Métriques', to: '/metrics', icon: Activity },
  { label: 'Journaux système', to: '/logs', icon: ScrollText },
  { label: 'Services système', to: '/services', icon: Wrench },
  { label: 'Sauvegardes', to: '/backups', icon: DatabaseBackup },
  { label: 'Tâches planifiées', to: '/cron', icon: CalendarClock },
]

const activityItems: NavItem[] = [
  { label: 'Alertes', to: '/alerts', icon: Bell },
  { label: 'Événements', to: '/events', icon: Gauge },
]

function navLinkClass({ isActive }: { isActive: boolean }) {
  return cn(
    'flex h-8 items-center gap-2.5 rounded-md px-2.5 text-[13.5px] transition-colors',
    isActive
      ? 'bg-elevated font-medium text-text-primary'
      : 'text-text-secondary hover:bg-elevated/60 hover:text-text-primary',
  )
}

function NavSection({
  title,
  items,
  onNavigate,
}: {
  title: string
  items: NavItem[]
  onNavigate: () => void
}) {
  return (
    <div>
      <p className="mb-1.5 px-2.5 text-xs text-text-muted">{title}</p>
      <div className="space-y-0.5">
        {items.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.end}
            className={navLinkClass}
            onClick={onNavigate}
          >
            <item.icon className="h-4 w-4 shrink-0" aria-hidden="true" />
            <span className="truncate">{item.label}</span>
          </NavLink>
        ))}
      </div>
    </div>
  )
}

type AppSidebarProps = {
  open: boolean
  onClose: () => void
}

export function AppSidebar({ open, onClose }: AppSidebarProps) {
  const { host, online, osLabel } = useHost()
  const { canManageUsers } = useSession()

  return (
    <>
      <div
        className={cn(
          'fixed inset-0 z-40 bg-black/60 backdrop-blur-sm transition-opacity lg:hidden',
          open ? 'opacity-100' : 'pointer-events-none opacity-0',
        )}
        onClick={onClose}
        aria-hidden="true"
      />
      <aside
        className={cn(
          'fixed inset-y-0 left-0 z-50 flex w-60 flex-col border-r border-line bg-surface transition-transform duration-200 lg:translate-x-0',
          open ? 'translate-x-0' : '-translate-x-full',
        )}
        aria-label="Navigation principale"
      >
        <div className="flex h-14 items-center justify-between px-4">
          <Logo />
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1.5 text-text-muted transition-colors hover:bg-elevated hover:text-text-primary lg:hidden"
            aria-label="Fermer le menu"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <nav className="flex-1 space-y-5 overflow-y-auto px-3 pb-4 pt-2">
          <div>
            <NavLink to="/" end className={navLinkClass} onClick={onClose}>
              <LayoutDashboard className="h-4 w-4 shrink-0" aria-hidden="true" />
              <span>Vue d'ensemble</span>
            </NavLink>
          </div>

          <NavSection title="Docker" items={dockerItems} onNavigate={onClose} />
          <NavSection title="Système" items={systemItems} onNavigate={onClose} />
          <NavSection title="Activité" items={activityItems} onNavigate={onClose} />

          <div>
            <p className="mb-1.5 px-2.5 text-xs text-text-muted">Paramètres</p>
            <NavLink to="/settings" className={navLinkClass} onClick={onClose}>
              <Settings className="h-4 w-4 shrink-0" aria-hidden="true" />
              <span>Configuration</span>
            </NavLink>
            {canManageUsers && (
              <NavLink to="/users" className={navLinkClass} onClick={onClose}>
                <Users className="h-4 w-4 shrink-0" aria-hidden="true" />
                <span>Utilisateurs & accès</span>
              </NavLink>
            )}
          </div>
        </nav>

        <div className="space-y-2.5 border-t border-line px-5 py-4">
          <div className="flex items-center gap-2.5">
            <StatusDot
              tone={online ? 'success' : 'warning'}
              label={online ? 'Collecte système active' : 'Collecte système dégradée'}
            />
            <div className="min-w-0">
              <p className="truncate font-mono text-[13px] text-text-primary">
                {host?.hostname ?? 'Machine locale'}
              </p>
              <p className="truncate text-xs text-text-muted">
                {host ? osLabel : 'Connexion au Manager…'}
              </p>
            </div>
          </div>
          <p className="text-xs text-text-muted">
            {online ? 'Collecte active' : 'Collecte locale'} · uptime{' '}
            {host ? formatUptime(host.uptimeSeconds) : '—'} · v{APP_VERSION}
          </p>
        </div>
      </aside>
    </>
  )
}
