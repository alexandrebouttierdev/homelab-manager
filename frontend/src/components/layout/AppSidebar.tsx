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

type SoonItem = {
  label: string
  icon: LucideIcon
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
]

const activityItems: NavItem[] = [{ label: 'Événements', to: '/events', icon: Gauge }]

const soonItems: SoonItem[] = [
  { label: 'Alertes', icon: Bell },
  { label: 'Sauvegardes', icon: DatabaseBackup },
  { label: 'Tâches Cron', icon: CalendarClock },
]

function navLinkClass({ isActive }: { isActive: boolean }) {
  return cn(
    'group flex items-center gap-2.5 rounded-md px-2.5 py-2 text-[13px] transition-colors',
    isActive
      ? 'bg-elevated text-text-primary shadow-sm'
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
      <p className="mb-1.5 px-2.5 font-mono text-[10px] font-medium uppercase tracking-wider text-text-muted">
        {title}
      </p>
      <div className="space-y-0.5">
        {items.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.end}
            className={navLinkClass}
            onClick={onNavigate}
          >
            <item.icon
              className="h-4 w-4 shrink-0 text-text-muted group-hover:text-text-secondary"
              aria-hidden="true"
            />
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
          'fixed inset-y-0 left-0 z-50 flex w-64 flex-col border-r border-line bg-surface transition-transform duration-200 lg:translate-x-0',
          open ? 'translate-x-0' : '-translate-x-full',
        )}
        aria-label="Navigation principale"
      >
        <div className="flex h-14 items-center justify-between border-b border-line px-4">
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

        <nav className="flex-1 space-y-5 overflow-y-auto px-3 py-4">
          <div>
            <NavLink to="/" end className={navLinkClass} onClick={onClose}>
              <LayoutDashboard className="h-4 w-4 shrink-0 text-text-muted" aria-hidden="true" />
              <span>Vue d'ensemble</span>
            </NavLink>
          </div>

          <NavSection title="Docker" items={dockerItems} onNavigate={onClose} />
          <NavSection title="Système" items={systemItems} onNavigate={onClose} />
          <NavSection title="Activité" items={activityItems} onNavigate={onClose} />

          <div>
            <p className="mb-1.5 px-2.5 font-mono text-[10px] font-medium uppercase tracking-wider text-text-muted">
              Paramètres
            </p>
            <NavLink to="/settings" className={navLinkClass} onClick={onClose}>
              <Settings className="h-4 w-4 shrink-0 text-text-muted" aria-hidden="true" />
              <span>Configuration</span>
            </NavLink>
            {canManageUsers && (
              <NavLink to="/users" className={navLinkClass} onClick={onClose}>
                <Users className="h-4 w-4 shrink-0 text-text-muted" aria-hidden="true" />
                <span>Utilisateurs & accès</span>
              </NavLink>
            )}
          </div>

          <div>
            <p className="mb-1.5 px-2.5 font-mono text-[10px] font-medium uppercase tracking-wider text-text-muted/70">
              Feuille de route
            </p>
            <div className="space-y-0.5">
              {soonItems.map((item) => (
                <span
                  key={item.label}
                  title="Fonctionnalité non disponible dans cette version"
                  className="flex cursor-not-allowed items-center gap-2.5 rounded-md px-2.5 py-1.5 text-[12px] text-text-muted/60"
                >
                  <item.icon className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                  <span className="truncate">{item.label}</span>
                  <span className="ml-auto rounded-sm border border-line/60 px-1 font-mono text-[9px] uppercase tracking-wide">
                    à venir
                  </span>
                </span>
              ))}
            </div>
          </div>
        </nav>

        <div className="border-t border-line px-4 py-3">
          <div className="flex items-center gap-2">
            <StatusDot
              tone={online ? 'success' : 'warning'}
              pulse={online}
              label={online ? 'Collecte système active' : 'Collecte système dégradée'}
            />
            <span className="font-mono text-[10px] uppercase tracking-wider text-text-secondary">
              {online ? 'Collecte active' : 'Collecte locale'}
            </span>
          </div>
          <p className="mt-2 truncate text-xs font-medium text-text-primary">
            {host?.hostname ?? 'Homelab local'}
          </p>
          <p className="mt-0.5 truncate font-mono text-[10px] text-text-muted">
            {host ? osLabel : 'Connexion au Manager…'}
          </p>
          <p className="mt-1 font-mono text-[10px] text-text-muted">
            Uptime {host ? formatUptime(host.uptimeSeconds) : '—'}
          </p>
          <p className="mt-2 font-mono text-[10px] uppercase tracking-wider text-text-muted/70">
            Version {APP_VERSION}
          </p>
        </div>
      </aside>
    </>
  )
}
