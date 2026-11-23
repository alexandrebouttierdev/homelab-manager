import { LogOut } from 'lucide-react'
import { Navigate } from 'react-router-dom'
import { Button } from '../../components/ui/Button'
import { Logo } from '../../components/ui/Logo'
import { SecondFactorCard } from '../settings/SecondFactorCard'
import { useSecondFactorSetupPage } from './useSecondFactorSetupPage'

export function SecondFactorSetupPage() {
  const page = useSecondFactorSetupPage()

  if (page.redirectTo) return <Navigate to={page.redirectTo} replace />

  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-6 bg-background px-4 py-8">
      <Logo />
      <div className="w-full max-w-2xl space-y-4">
        <div className="text-center">
          <h1 className="text-lg font-semibold text-text-primary">
            Activez la double authentification
          </h1>
          <p className="mt-1 text-sm text-text-secondary">
            La politique de sécurité l'exige pour les administrateurs
            {page.username ? ` (compte ${page.username})` : ''}.
          </p>
        </div>
        <SecondFactorCard model={page.secondFactor} />
        <div className="text-center">
          <Button
            size="sm"
            variant="ghost"
            icon={<LogOut className="h-3.5 w-3.5" />}
            onClick={page.signOut}
          >
            Se déconnecter
          </Button>
        </div>
      </div>
    </main>
  )
}
