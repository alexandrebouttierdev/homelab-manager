import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Bell,
  Clock3,
  LogOut,
  Menu,
  Moon,
  Power,
  RefreshCw,
  Server,
  Sun,
  Terminal,
  UserRound,
} from 'lucide-react'
import { Link, useNavigate } from 'react-router-dom'
import { getSession, logout } from '../../lib/api/auth'
import { getHost } from '../../lib/api/monitoring'
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
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const { interval, setInterval } = useRefreshControl()
  const { theme, toggleTheme } = useTheme()

  const session = useQuery({
    queryKey: ['session'],
    queryFn: getSession,
    staleTime: 60_000,
    retry: false,
  })
  const host = useQuery({
    queryKey: ['host'],
    queryFn: getHost,
    staleTime: 30_000,
    retry: false,
  })
  const signOut = useMutation({
    mutationFn: logout,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['session'] })
      navigate('/login', { replace: true })
    },
  })

  const hostInfo = host.data
  const hostOnline = hostInfo?.collector.status === 'ok'

  return (
    <>
      <header className="sticky top-0 z-30 flex h-14 items-center justify-between gap-3 border-b border-line bg-background/85 px-3 backdrop-blur-md sm:px-4 lg:px-6">
        <div className="flex min-w-0 items-center gap-2.5">
          <button
            type="button"
            onClick={onOpenMenu}
            className="rounded-md p-1.5 text-text-secondary transition-colors hover:bg-elevated hover:text-text-primary lg:hidden"
            aria-label="Ouvrir le menu"
          >
            <Menu className="h-5 w-5" />
          </button>

          <div className="hidden items-center gap-2 rounded-md border border-line bg-card px-2.5 py-1.5 sm:flex">
            <Server className="h-3.5 w-3.5 text-text-muted" aria-hidden="true" />
            <span className="max-w-[160px] truncate font-mono text-[11px] text-text-secondary">
              {hostInfo?.hostname ?? 'hôte local'}
            </span>
            <StatusDot tone={hostOnline ? 'success' : 'warning'} label={hostOnline ? 'Hôte en ligne' : 'Collecte dégradée'} />
          </div>

          <div className="hidden items-center gap-1.5 font-mono text-[11px] text-text-muted xl:flex">
            <Terminal className="h-3.5 w-3.5" aria-hidden="true" />
            <span className="max-w-[180px] truncate">
              {hostInfo
                ? `${hostInfo.os.distribution || 'Linux'} ${hostInfo.os.version || ''}`.trim()
                : 'Linux'}
            </span>
            <span className="text-line-strong">•</span>
            <span className="max-w-[160px] truncate">{hostInfo?.kernelVersion || 'noyau indisponible'}</span>
          </div>

          <div className="hidden items-center gap-1.5 font-mono text-[11px] text-text-muted md:flex">
            <Clock3 className="h-3.5 w-3.5" aria-hidden="true" />
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
              className="h-8 w-[92px] font-mono text-[11px]"
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
            className="relative rounded-md border border-line bg-card p-1.5 text-text-secondary transition-colors hover:bg-elevated hover:text-text-primary"
            aria-label="Consulter les événements"
            title="Événements"
          >
            <Bell className="h-4 w-4" />
          </Link>

          <button
            type="button"
            onClick={toggleTheme}
            className="rounded-md border border-line bg-card p-1.5 text-text-secondary transition-colors hover:bg-elevated hover:text-text-primary"
            aria-label={theme === 'dark' ? 'Activer le thème clair' : 'Activer le thème sombre'}
            title={theme === 'dark' ? 'Thème clair' : 'Thème sombre'}
          >
            {theme === 'dark' ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
          </button>

          <button
            type="button"
            onClick={() => signOut.mutate()}
            disabled={signOut.isPending}
            className={cn(
              'rounded-md border border-line bg-card p-1.5 text-text-secondary transition-colors hover:bg-danger/10 hover:text-danger',
              signOut.isPending && 'cursor-wait opacity-50',
            )}
            aria-label="Se déconnecter"
            title="Se déconnecter"
          >
            <Power className="h-4 w-4" />
          </button>

          <Link
            to="/settings"
            className="ml-0.5 hidden items-center gap-1.5 rounded-md border border-line bg-card px-2.5 py-1.5 text-xs text-text-secondary transition-colors hover:bg-elevated hover:text-text-primary sm:flex"
            title="Compte et paramètres"
          >
            <UserRound className="h-3.5 w-3.5" aria-hidden="true" />
            <span className="max-w-[110px] truncate font-mono text-[11px]">
              {session.data?.user.username ?? 'admin'}
            </span>
          </Link>
        </div>
      </header>

      {signOut.isError && (
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
