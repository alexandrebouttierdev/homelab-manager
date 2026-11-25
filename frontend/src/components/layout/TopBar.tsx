import { Bell, LogOut, Menu, Moon, Power, RefreshCw, Sun, UserRound } from 'lucide-react'
import { Link } from 'react-router-dom'
import { useHost } from '../../hooks/useHost'
import { useSession, useSignOut } from '../../hooks/useSession'
import { cn } from '../../lib/cn'
import { formatUptime } from '../../lib/format'
import { refreshChoices, useRefreshControl } from '../../lib/refresh'
import { useTheme } from '../../lib/theme'
import { Select } from '../ui/Select'
import { StatusDot } from '../ui/StatusDot'

type TopBarProps = {
  onOpenMenu: () => void
}

export function TopBar({ onOpenMenu }: TopBarProps) {
  const { interval, setInterval } = useRefreshControl()
  const { theme, toggleTheme } = useTheme()
  const session = useSession()
  const { host: hostInfo, online: hostOnline, osLabel } = useHost()
  const signOut = useSignOut()

  return (
    <>
      <header className="sticky top-0 z-30 flex h-14 items-center justify-between gap-3 border-b border-line bg-background/90 px-3 backdrop-blur-md sm:px-4 lg:px-8">
        <div className="flex min-w-0 items-center gap-2.5">
          <button
            type="button"
            onClick={onOpenMenu}
            className="rounded-md p-1.5 text-text-secondary transition-colors hover:bg-elevated hover:text-text-primary lg:hidden"
            aria-label="Ouvrir le menu"
          >
            <Menu className="h-5 w-5" />
          </button>

          <div className="hidden min-w-0 items-center gap-2.5 sm:flex">
            <StatusDot
              tone={hostOnline ? 'success' : 'warning'}
              label={hostOnline ? 'Hôte en ligne' : 'Collecte dégradée'}
            />
            <span className="max-w-[160px] truncate font-mono text-[13px] text-text-primary">
              {hostInfo?.hostname ?? 'hôte local'}
            </span>
          </div>

          <div className="hidden min-w-0 items-center gap-2 text-[13px] text-text-muted xl:flex">
            <span className="text-line-strong" aria-hidden="true">
              /
            </span>
            <span className="max-w-[180px] truncate">{hostInfo ? osLabel : 'Linux'}</span>
            <span aria-hidden="true">·</span>
            <span className="max-w-[160px] truncate">
              {hostInfo?.kernelVersion || 'noyau indisponible'}
            </span>
          </div>

          <div className="hidden items-center gap-2 text-[13px] text-text-muted md:flex">
            <span aria-hidden="true">·</span>
            <span>Up {hostInfo ? formatUptime(hostInfo.uptimeSeconds) : '—'}</span>
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-1.5">
          <div className="hidden items-center gap-1.5 sm:flex">
            <RefreshCw className="h-3.5 w-3.5 text-text-muted" aria-hidden="true" />
            <Select
              aria-label="Fréquence de rafraîchissement"
              value={interval}
              onChange={(event) => setInterval(Number(event.target.value))}
              className="h-8 w-[100px]"
            >
              {refreshChoices.map((choice) => (
                <option key={choice.value} value={choice.value}>
                  {choice.value === 0 ? 'Pause' : `Auto ${choice.label}`}
                </option>
              ))}
            </Select>
          </div>

          <Link
            to="/events"
            className="relative inline-flex h-8 w-8 items-center justify-center rounded-md text-text-secondary transition-colors hover:bg-elevated hover:text-text-primary"
            aria-label="Consulter les événements"
            title="Événements"
          >
            <Bell className="h-4 w-4" />
          </Link>

          <button
            type="button"
            onClick={toggleTheme}
            className="inline-flex h-8 w-8 items-center justify-center rounded-md text-text-secondary transition-colors hover:bg-elevated hover:text-text-primary"
            aria-label={theme === 'dark' ? 'Activer le thème clair' : 'Activer le thème sombre'}
            title={theme === 'dark' ? 'Thème clair' : 'Thème sombre'}
          >
            {theme === 'dark' ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
          </button>

          <button
            type="button"
            onClick={signOut.signOut}
            disabled={signOut.isPending}
            className={cn(
              'inline-flex h-8 w-8 items-center justify-center rounded-md text-text-secondary transition-colors hover:bg-danger/10 hover:text-danger',
              signOut.isPending && 'cursor-wait opacity-50',
            )}
            aria-label="Se déconnecter"
            title="Se déconnecter"
          >
            <Power className="h-4 w-4" />
          </button>

          <Link
            to="/settings"
            className="ml-1 hidden h-8 items-center gap-2 rounded-md pl-1 pr-2.5 text-[13px] text-text-secondary transition-colors hover:bg-elevated hover:text-text-primary sm:flex"
            title="Compte et paramètres"
          >
            <span className="flex h-6 w-6 items-center justify-center rounded-full bg-elevated text-text-primary">
              <UserRound className="h-3.5 w-3.5" aria-hidden="true" />
            </span>
            <span className="max-w-[110px] truncate">{session.user?.username ?? 'admin'}</span>
          </Link>
        </div>
      </header>

      {signOut.error && (
        <div
          className="fixed bottom-4 right-4 z-50 animate-toast-in rounded-lg border border-danger/30 bg-card px-4 py-3 text-sm text-danger shadow-xl"
          role="alert"
        >
          <div className="flex items-center gap-2">
            <LogOut className="h-4 w-4" />
            <span>{signOut.error.message}</span>
          </div>
        </div>
      )}
    </>
  )
}
