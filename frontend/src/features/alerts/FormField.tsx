import type { ReactNode } from 'react'

export const inputClass =
  'h-9 w-full rounded-md border border-line bg-surface px-3 text-sm text-text-primary outline-none focus:border-accent'

/** A labelled field of the alerts forms, with its error under it. */
export function FormField({
  id,
  label,
  error,
  hint,
  className,
  children,
}: {
  id: string
  label: string
  error?: string
  hint?: string
  className?: string
  children: ReactNode
}) {
  return (
    <div className={className}>
      <label htmlFor={id} className="mb-1.5 block text-xs text-text-secondary">
        {label}
      </label>
      {children}
      {hint && !error && <p className="mt-1 text-xs text-text-muted">{hint}</p>}
      {error && <p className="mt-1 text-xs text-danger">{error}</p>}
    </div>
  )
}
