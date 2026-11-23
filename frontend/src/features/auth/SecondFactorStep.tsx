import { ShieldCheck } from 'lucide-react'
import type { useLogin } from './useLogin'

type SecondFactorStepProps = {
  step: NonNullable<ReturnType<typeof useLogin>['secondFactor']>
}

const linkClass = 'text-xs text-accent underline-offset-2 hover:underline'

export function SecondFactorStep({ step }: SecondFactorStepProps) {
  const totp = step.method === 'totp'

  return (
    <form className="mt-6 space-y-4" onSubmit={step.onSubmit} noValidate>
      <div className="flex items-start gap-2 rounded-md border border-line/70 bg-surface/60 px-3 py-2.5">
        <ShieldCheck className="mt-0.5 h-3.5 w-3.5 shrink-0 text-success" aria-hidden="true" />
        <p className="text-xs leading-relaxed text-text-secondary">
          {totp
            ? "Saisissez le code à 6 chiffres de votre application d'authentification."
            : 'Saisissez un de vos codes de secours. Il ne servira plus ensuite.'}
        </p>
      </div>
      <div>
        <label
          htmlFor="secondFactorCode"
          className="mb-1.5 block font-mono text-[10px] font-medium uppercase tracking-wider text-text-secondary"
        >
          {totp ? "Code de l'application" : 'Code de secours'}
        </label>
        <input
          id="secondFactorCode"
          autoFocus
          inputMode={totp ? 'numeric' : 'text'}
          autoComplete="one-time-code"
          placeholder={totp ? '123 456' : 'xxxx-xxxx-xxxx'}
          className="h-10 w-full rounded-md border border-line bg-surface px-3 font-mono text-sm text-text-primary outline-none placeholder:text-text-muted focus:border-accent/60"
          {...step.registerCode('code')}
        />
        {step.codeError && (
          <p className="mt-1 text-xs text-danger" role="alert">
            {step.codeError}
          </p>
        )}
      </div>
      {step.errorMessage && (
        <div
          className="rounded-md border border-danger/30 bg-danger/10 px-3 py-2 text-xs text-danger"
          role="alert"
        >
          {step.errorMessage}
        </div>
      )}
      <button
        type="submit"
        disabled={step.isPending}
        className="flex h-10 w-full items-center justify-center rounded-md bg-accent px-4 text-sm font-medium text-on-accent transition-colors hover:bg-accent/90 disabled:cursor-wait disabled:opacity-60"
      >
        {step.isPending ? 'Vérification…' : 'Valider'}
      </button>
      <div className="flex flex-wrap justify-between gap-2">
        <button
          type="button"
          className={linkClass}
          onClick={totp ? step.useRecoveryCode : step.useApp}
        >
          {totp ? 'Utiliser un code de secours' : "Utiliser l'application"}
        </button>
        <button type="button" className={linkClass} onClick={step.restart}>
          Recommencer
        </button>
      </div>
    </form>
  )
}
