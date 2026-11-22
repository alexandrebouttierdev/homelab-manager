import { Search, X } from 'lucide-react'
import { cn } from '../../lib/cn'

type SearchFieldProps = {
  value: string
  onChange: (value: string) => void
  placeholder?: string
  ariaLabel?: string
  className?: string
}

export function SearchField({
  value,
  onChange,
  placeholder = 'Rechercher…',
  ariaLabel = 'Rechercher',
  className,
}: SearchFieldProps) {
  return (
    <div
      className={cn(
        'flex h-9 min-w-0 items-center gap-2 rounded-md border border-line bg-card px-3 text-text-secondary transition-colors focus-within:border-accent/60',
        className,
      )}
    >
      <Search className="h-4 w-4 shrink-0 text-text-muted" aria-hidden="true" />
      <input
        type="search"
        aria-label={ariaLabel}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        className="min-w-0 flex-1 bg-transparent text-sm text-text-primary outline-none placeholder:text-text-muted [&::-webkit-search-cancel-button]:hidden"
      />
      {value && (
        <button
          type="button"
          aria-label="Effacer la recherche"
          onClick={() => onChange('')}
          className="rounded-sm p-0.5 text-text-muted transition-colors hover:text-text-primary"
        >
          <X className="h-3.5 w-3.5" />
        </button>
      )}
    </div>
  )
}
