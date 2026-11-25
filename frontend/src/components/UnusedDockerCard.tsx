import { Archive } from 'lucide-react'
import { Card, CardHeader } from './ui/Card'
import { formatBytes, formatDateTime } from '../lib/format'
import type { UnusedSection } from '../lib/dockerInventory'

type UnusedDockerCardProps = {
  title: string
  /** Shown when no item is unused. */
  emptyLabel: string
  section: UnusedSection
}

/** Images, volumes or networks that no container uses, read from Docker. */
export function UnusedDockerCard({ title, emptyLabel, section }: UnusedDockerCardProps) {
  const { items, notice, totalBytes } = section
  const subtitle =
    items === null
      ? 'inventaire Docker'
      : `${items.length} élément(s)${totalBytes ? ` · ${formatBytes(totalBytes)}` : ''}`

  return (
    <Card aria-label={title}>
      <CardHeader title={title} subtitle={subtitle} icon={<Archive className="h-3.5 w-3.5" />} />
      {notice && <p className="border-b border-line/70 px-4 py-2 text-xs text-warning">{notice}</p>}
      {items !== null && items.length === 0 && (
        <p className="px-4 py-3 text-xs text-text-muted">{emptyLabel}</p>
      )}
      {items !== null && items.length > 0 && (
        <ul className="divide-y divide-line/70">
          {items.map((item) => (
            <li
              key={item.key}
              className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1 px-4 py-2"
            >
              <span className="min-w-0 break-all font-mono text-xs text-text-primary">
                {item.name}
              </span>
              <span className="font-mono text-[10px] text-text-muted">
                {item.detail}
                {item.createdAt ? ` · créé le ${formatDateTime(item.createdAt)}` : ''}
              </span>
            </li>
          ))}
        </ul>
      )}
    </Card>
  )
}
