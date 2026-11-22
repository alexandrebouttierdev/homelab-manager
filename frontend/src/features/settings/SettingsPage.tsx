import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Clock3,
  Database,
  KeyRound,
  LogOut,
  MonitorCog,
  Moon,
  Palette,
  RefreshCw,
  ShieldCheck,
  Sun,
  UserRound,
} from 'lucide-react'
import { useForm } from 'react-hook-form'
import { useNavigate } from 'react-router-dom'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardFooter, CardHeader } from '../../components/ui/Card'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { PageHeader } from '../../components/ui/PageHeader'
import { StatusDot } from '../../components/ui/StatusDot'
import { ApiError, changePassword, getSession, logout } from '../../lib/api/auth'
import { getSettings } from '../../lib/api/monitoring'
import { cn } from '../../lib/cn'
import { formatRelativeTime } from '../../lib/format'
import { useTheme, type Theme } from '../../lib/theme'
import { passwordSchema, type PasswordValues } from '../../schemas/password'
import { APP_VERSION } from '../../lib/version'
import { RetentionForm } from './RetentionForm'

export function SettingsPage() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const { theme, setTheme } = useTheme()

  const settings = useQuery({ queryKey: ['settings'], queryFn: getSettings })
  const session = useQuery({ queryKey: ['session'], queryFn: getSession, staleTime: 60_000 })

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<PasswordValues>({
    resolver: zodResolver(passwordSchema),
    defaultValues: { currentPassword: '', newPassword: '', confirmPassword: '' },
  })

  const password = useMutation({
    mutationFn: (values: PasswordValues) =>
      changePassword(values.currentPassword, values.newPassword),
    // The Manager revokes every session, including this one, after a change.
    onSuccess: () => {
      reset()
      queryClient.removeQueries({ queryKey: ['session'] })
      navigate('/login', {
        replace: true,
        state: { notice: 'Mot de passe modifié. Reconnectez-vous avec le nouveau mot de passe.' },
      })
    },
  })
  const signOut = useMutation({
    mutationFn: logout,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['session'] })
      navigate('/login', { replace: true })
    },
  })

  const passwordError =
    password.error instanceof ApiError && password.error.status === 401
      ? 'Le mot de passe actuel est incorrect.'
      : password.error?.message

  return (
    <div className="space-y-4">
      <PageHeader
        title="Paramètres"
        breadcrumb={[{ label: 'Paramètres' }, { label: 'Configuration' }]}
        badge={<Badge tone="neutral">v{APP_VERSION}</Badge>}
        description="Configuration effective du Manager, apparence de l'interface et sécurité du compte administrateur local."
      />

      {settings.isError && (
        <ErrorState
          message={settings.error.message}
          onRetry={() => {
            void settings.refetch()
          }}
        />
      )}
      {settings.isLoading && !settings.isError && (
        <LoadingState label="Chargement de la configuration…" rows={2} />
      )}

      {settings.data && (
        <>
          <div className="grid items-start gap-4 xl:grid-cols-3">
            <Card className="xl:col-span-2">
              <CardHeader
                title="Supervision"
                subtitle="Valeurs actives du service"
                icon={<MonitorCog className="h-3.5 w-3.5" />}
              />
              <CardBody>
                <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                  <SettingCard
                    label="Collecte système"
                    value={`${settings.data.systemCollectIntervalSeconds} s`}
                    icon={<Clock3 className="h-3.5 w-3.5" />}
                  />
                  <SettingCard
                    label="Statistiques Docker"
                    value={`${settings.data.dockerCollectIntervalSeconds} s`}
                    icon={<RefreshCw className="h-3.5 w-3.5" />}
                  />
                  <SettingCard
                    label="Découverte Docker"
                    value={`${settings.data.dockerDiscoveryIntervalSeconds} s`}
                    icon={<Database className="h-3.5 w-3.5" />}
                  />
                  <SettingCard
                    label="Rétention des métriques"
                    value={`${settings.data.metricsRetentionDays} jours`}
                    icon={<Clock3 className="h-3.5 w-3.5" />}
                  />
                </div>
              </CardBody>
              <CardFooter className="font-mono text-[10px] text-text-muted">
                Les fréquences de collecte sont définies dans la configuration du service et
                nécessitent un redémarrage. La rétention se règle ci-dessous, sans redémarrage.
              </CardFooter>
            </Card>

            <RetentionForm settings={settings.data} />

            <Card>
              <CardHeader
                title="Collecteurs"
                subtitle="État de la supervision locale"
                icon={<ShieldCheck className="h-3.5 w-3.5" />}
              />
              <CardBody className="space-y-3">
                <CollectorRow
                  name="Système"
                  status={settings.data.collectors.system.status}
                  lastSuccessAt={settings.data.collectors.system.lastSuccessAt}
                />
                <CollectorRow
                  name="Docker"
                  status={settings.data.collectors.docker.status}
                  lastSuccessAt={settings.data.collectors.docker.lastSuccessAt}
                />
                <div className="rounded-md border border-line/70 bg-surface/50 px-3 py-2.5 font-mono text-[10px] text-text-muted">
                  <p>Le Manager supervise uniquement cette machine.</p>
                  <p className="mt-1">Aucun agent distant n'est requis.</p>
                </div>
              </CardBody>
            </Card>
          </div>

          <div className="grid items-start gap-4 xl:grid-cols-2">
            <Card>
              <CardHeader
                title="Apparence"
                subtitle="Thème de l'interface"
                icon={<Palette className="h-3.5 w-3.5" />}
              />
              <CardBody>
                <div className="grid gap-3 sm:grid-cols-2">
                  <ThemeChoice
                    label="Sombre minimal"
                    description="Fond graphite et accents indigo."
                    theme="dark"
                    selected={theme === 'dark'}
                    onSelect={setTheme}
                  />
                  <ThemeChoice
                    label="Clair atelier"
                    description="Fond clair et contrastes renforcés."
                    theme="light"
                    selected={theme === 'light'}
                    onSelect={setTheme}
                  />
                </div>
              </CardBody>
              <CardFooter className="font-mono text-[10px] text-text-muted">
                Le choix est mémorisé dans ce navigateur.
              </CardFooter>
            </Card>

            <Card>
              <CardHeader
                title="Compte administrateur"
                subtitle="Session locale"
                icon={<UserRound className="h-3.5 w-3.5" />}
              />
              <CardBody className="space-y-3">
                <div className="flex items-center justify-between gap-3 rounded-md border border-line/70 bg-surface/50 px-3 py-2.5">
                  <div className="flex items-center gap-2">
                    <span className="flex h-7 w-7 items-center justify-center rounded-md border border-line bg-elevated text-text-muted">
                      <UserRound className="h-3.5 w-3.5" />
                    </span>
                    <div>
                      <p className="text-xs font-medium text-text-primary">
                        {session.data?.user.username ?? 'administrateur'}
                      </p>
                      <p className="font-mono text-[10px] text-text-muted">Session authentifiée</p>
                    </div>
                  </div>
                  <Badge tone="success" dot>
                    Active
                  </Badge>
                </div>
                <p className="text-xs leading-relaxed text-text-secondary">
                  La session est protégée par un cookie HTTP-only. La déconnexion révoque
                  immédiatement la session côté Manager.
                </p>
                <Button
                  variant="danger"
                  size="sm"
                  icon={<LogOut className="h-3.5 w-3.5" />}
                  loading={signOut.isPending}
                  onClick={() => signOut.mutate()}
                >
                  Se déconnecter
                </Button>
                {signOut.isError && (
                  <p className="text-xs text-danger" role="alert">
                    {signOut.error.message}
                  </p>
                )}
              </CardBody>
            </Card>
          </div>

          <Card>
            <CardHeader
              title="Changer le mot de passe"
              subtitle="Minimum 12 caractères"
              icon={<KeyRound className="h-3.5 w-3.5" />}
            />
            <CardBody>
              <form
                className="grid max-w-2xl gap-4 sm:grid-cols-2"
                onSubmit={handleSubmit((values) => password.mutate(values))}
                noValidate
              >
                <div className="sm:col-span-2">
                  <label
                    htmlFor="currentPassword"
                    className="mb-1.5 block font-mono text-[10px] uppercase tracking-wider text-text-secondary"
                  >
                    Mot de passe actuel
                  </label>
                  <input
                    id="currentPassword"
                    type="password"
                    autoComplete="current-password"
                    className="h-10 w-full rounded-md border border-line bg-surface px-3 text-sm text-text-primary outline-none transition-colors focus:border-accent/60"
                    {...register('currentPassword')}
                  />
                  {errors.currentPassword && (
                    <p className="mt-1 text-xs text-danger">{errors.currentPassword.message}</p>
                  )}
                </div>
                <div>
                  <label
                    htmlFor="newPassword"
                    className="mb-1.5 block font-mono text-[10px] uppercase tracking-wider text-text-secondary"
                  >
                    Nouveau mot de passe
                  </label>
                  <input
                    id="newPassword"
                    type="password"
                    autoComplete="new-password"
                    className="h-10 w-full rounded-md border border-line bg-surface px-3 text-sm text-text-primary outline-none transition-colors focus:border-accent/60"
                    {...register('newPassword')}
                  />
                  {errors.newPassword && (
                    <p className="mt-1 text-xs text-danger">{errors.newPassword.message}</p>
                  )}
                </div>
                <div>
                  <label
                    htmlFor="confirmPassword"
                    className="mb-1.5 block font-mono text-[10px] uppercase tracking-wider text-text-secondary"
                  >
                    Confirmer le mot de passe
                  </label>
                  <input
                    id="confirmPassword"
                    type="password"
                    autoComplete="new-password"
                    className="h-10 w-full rounded-md border border-line bg-surface px-3 text-sm text-text-primary outline-none transition-colors focus:border-accent/60"
                    {...register('confirmPassword')}
                  />
                  {errors.confirmPassword && (
                    <p className="mt-1 text-xs text-danger">{errors.confirmPassword.message}</p>
                  )}
                </div>

                {passwordError && (
                  <div
                    className="rounded-md border border-danger/30 bg-danger/10 px-3 py-2 text-xs text-danger sm:col-span-2"
                    role="alert"
                  >
                    {passwordError}
                  </div>
                )}

                <div className="sm:col-span-2">
                  <Button
                    type="submit"
                    variant="primary"
                    loading={password.isPending}
                    icon={<KeyRound className="h-4 w-4" />}
                  >
                    Enregistrer le mot de passe
                  </Button>
                </div>
              </form>
            </CardBody>
            <CardFooter className="font-mono text-[10px] text-text-muted">
              Le mot de passe est haché avec Argon2id côté serveur et n'est jamais journalisé.
            </CardFooter>
          </Card>
        </>
      )}

      <Card>
        <CardHeader
          title="À propos"
          subtitle="Homelab Manager · architecture mono-hôte"
          icon={<Database className="h-3.5 w-3.5" />}
        />
        <CardBody className="grid gap-3 text-xs text-text-secondary sm:grid-cols-2">
          <p>
            Backend Go, API REST Chi, SQLite et collecte locale des métriques système et Docker.
          </p>
          <p>
            Frontend React, TypeScript strict, Tailwind CSS et ECharts. Aucune dépendance cloud
            n'est requise pour le fonctionnement normal.
          </p>
          <p>
            Le Manager n'expose ni socket Docker, ni commande arbitraire, ni administration des
            conteneurs au navigateur.
          </p>
          <p>
            Les fonctionnalités non implémentées sont explicitement signalées comme indisponibles
            dans la navigation.
          </p>
        </CardBody>
        <CardFooter className="flex flex-wrap items-center justify-between gap-2 font-mono text-[10px] text-text-muted">
          <span>Version {APP_VERSION}</span>
          <span>Supervision locale uniquement</span>
        </CardFooter>
      </Card>
    </div>
  )
}

function SettingCard({
  label,
  value,
  icon,
}: {
  label: string
  value: string
  icon: React.ReactNode
}) {
  return (
    <div className="rounded-md border border-line/70 bg-surface/50 p-3">
      <div className="flex items-center gap-1.5 font-mono text-[10px] uppercase tracking-wider text-text-muted">
        {icon}
        {label}
      </div>
      <p className="mt-1.5 font-mono text-lg font-semibold text-text-primary">{value}</p>
    </div>
  )
}

function CollectorRow({
  name,
  status,
  lastSuccessAt,
}: {
  name: string
  status: string
  lastSuccessAt: string | null
}) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-md border border-line/70 bg-surface/50 px-3 py-2.5">
      <div className="flex items-center gap-2">
        <StatusDot tone={status === 'ok' ? 'success' : 'warning'} pulse={status === 'ok'} />
        <span className="text-xs font-medium text-text-primary">{name}</span>
      </div>
      <div className="text-right font-mono text-[10px] text-text-muted">
        <span className="block uppercase tracking-wider">{status}</span>
        <span>{lastSuccessAt ? formatRelativeTime(lastSuccessAt) : 'jamais'}</span>
      </div>
    </div>
  )
}

function ThemeChoice({
  label,
  description,
  theme,
  selected,
  onSelect,
}: {
  label: string
  description: string
  theme: Theme
  selected: boolean
  onSelect: (theme: Theme) => void
}) {
  return (
    <button
      type="button"
      onClick={() => onSelect(theme)}
      aria-pressed={selected}
      className={cn(
        'flex items-center gap-3 rounded-md border p-3 text-left transition-colors',
        selected
          ? 'border-accent/50 bg-accent/5'
          : 'border-line bg-surface/50 hover:border-line-strong hover:bg-elevated/50',
      )}
    >
      <span
        className={cn(
          'flex h-10 w-10 shrink-0 items-center justify-center rounded-md border',
          theme === 'dark'
            ? 'border-line-strong bg-[#111315] text-[#8b9cfb]'
            : 'border-[#dde1e7] bg-[#f3f4f6] text-[#4f5fd7]',
        )}
      >
        {theme === 'dark' ? <Moon className="h-4 w-4" /> : <Sun className="h-4 w-4" />}
      </span>
      <span className="min-w-0">
        <span className="flex items-center gap-2 text-xs font-medium text-text-primary">
          {label}
          {selected && <Badge tone="accent">Actif</Badge>}
        </span>
        <span className="mt-0.5 block text-[11px] text-text-muted">{description}</span>
      </span>
    </button>
  )
}
