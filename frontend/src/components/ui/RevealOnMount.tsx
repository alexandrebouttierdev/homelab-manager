import type { ReactNode } from 'react'

/**
 * Scrolls its content into view when it appears, if it is out of sight; a
 * panel opened below a long list stays visible on a narrow screen. Give it a
 * key to reveal it again when its subject changes.
 */
export function RevealOnMount({ children }: { children: ReactNode }) {
  return (
    <div
      className="scroll-mt-4"
      ref={(node) => node?.scrollIntoView?.({ block: 'nearest', behavior: 'smooth' })}
    >
      {children}
    </div>
  )
}
