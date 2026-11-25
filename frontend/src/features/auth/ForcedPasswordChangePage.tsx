import { KeyRound } from 'lucide-react'
import type { ReactNode } from 'react'
import { Navigate } from 'react-router-dom'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Logo } from '../../components/ui/Logo'
import { useForcedPasswordChange } from './useForcedPasswordChange'

const inputClass =
  'h-9 w-full rounded-md border border-line bg-surface px-3 text-sm text-text-primary outline-none focus:border-accent'

export function ForcedPasswordChangePage() {
  const page = useForcedPasswordChange()

  if (page.redirectTo) return <Navigate to={page.redirectTo} replace />

  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-6 bg-background px-4">
      <Logo />
      <Card className="w-full max-w-md">
        <CardHeader
          title="Choisissez votre mot de passe"
          subtitle={
            page.username
              ? `Compte ${page.username} : le mot de passe initial doit être remplacé`
              : 'Le mot de passe initial doit être remplacé'
          }
          icon={<KeyRound className="h-3.5 w-3.5" />}
        />
        <CardBody>
          <form className="space-y-3" onSubmit={page.onSubmit} noValidate>
            <Field
              id="currentPassword"
              label="Mot de passe initial"
              error={page.errors.currentPassword?.message}
            >
              <input
                id="currentPassword"
                type="password"
                autoComplete="current-password"
                className={inputClass}
                {...page.register('currentPassword')}
              />
            </Field>
            <Field
              id="newPassword"
              label="Nouveau mot de passe"
              error={page.errors.newPassword?.message}
            >
              <input
                id="newPassword"
                type="password"
                autoComplete="new-password"
                className={inputClass}
                {...page.register('newPassword')}
              />
            </Field>
            <Field
              id="confirmPassword"
              label="Confirmation"
              error={page.errors.confirmPassword?.message}
            >
              <input
                id="confirmPassword"
                type="password"
                autoComplete="new-password"
                className={inputClass}
                {...page.register('confirmPassword')}
              />
            </Field>
            {page.errorMessage && (
              <p className="text-xs text-danger" role="alert">
                {page.errorMessage}
              </p>
            )}
            <Button
              type="submit"
              variant="primary"
              loading={page.isSubmitting}
              disabled={page.checking}
            >
              Enregistrer
            </Button>
            <p className="text-xs text-text-muted">
              Minimum 12 caractères. Vous serez ensuite invité à vous reconnecter.
            </p>
          </form>
        </CardBody>
      </Card>
    </main>
  )
}

function Field({
  id,
  label,
  error,
  children,
}: {
  id: string
  label: string
  error?: string
  children: ReactNode
}) {
  return (
    <div>
      <label htmlFor={id} className="mb-1.5 block text-xs text-text-secondary">
        {label}
      </label>
      {children}
      {error && (
        <p className="mt-1 text-xs text-danger" role="alert">
          {error}
        </p>
      )}
    </div>
  )
}
