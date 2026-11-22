import type { HTMLAttributes, ReactNode, TdHTMLAttributes, ThHTMLAttributes } from 'react'
import { cn } from '../../lib/cn'

type TableProps = {
  children: ReactNode
  className?: string
  minWidthClassName?: string
}

export function Table({ children, className, minWidthClassName = 'min-w-[760px]' }: TableProps) {
  return (
    <div className={cn('overflow-x-auto rounded-lg border border-line bg-card', className)}>
      <table className={cn('w-full border-collapse text-left text-sm', minWidthClassName)}>
        {children}
      </table>
    </div>
  )
}

export function THead({ children }: { children: ReactNode }) {
  return <thead className="bg-surface/70">{children}</thead>
}

export function TBody({ children }: { children: ReactNode }) {
  return <tbody>{children}</tbody>
}

export function TR({ className, children, ...props }: HTMLAttributes<HTMLTableRowElement>) {
  return (
    <tr className={cn('transition-colors hover:bg-elevated/40', className)} {...props}>
      {children}
    </tr>
  )
}

export function TH({ className, children, ...props }: ThHTMLAttributes<HTMLTableCellElement>) {
  return (
    <th
      scope="col"
      className={cn('whitespace-nowrap px-4 py-2.5 font-mono text-[10px] font-medium uppercase tracking-wider text-text-muted', className)}
      {...props}
    >
      {children}
    </th>
  )
}

export function TD({ className, children, ...props }: TdHTMLAttributes<HTMLTableCellElement>) {
  return (
    <td className={cn('border-t border-line/70 px-4 py-3 align-middle text-text-secondary', className)} {...props}>
      {children}
    </td>
  )
}
