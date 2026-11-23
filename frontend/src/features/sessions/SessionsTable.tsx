import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/DataTable'
import type { SessionRow } from '../../lib/sessions'

type SessionsTableProps = {
  rows: SessionRow[]
  showUser: boolean
  onRevoke: (id: string) => void
  revoking: boolean
}

export function SessionsTable({ rows, showUser, onRevoke, revoking }: SessionsTableProps) {
  return (
    <Table minWidthClassName="min-w-[720px]">
      <THead>
        <TR>
          {showUser && <TH>Compte</TH>}
          <TH>Navigateur</TH>
          <TH>Adresse</TH>
          <TH>Méthode</TH>
          <TH>Dernière activité</TH>
          <TH>Ouverte le</TH>
          {/* relative: keeps the visually hidden label inside the scrolling table */}
          <TH className="relative">
            <span className="sr-only">Actions</span>
          </TH>
        </TR>
      </THead>
      <TBody>
        {rows.map((row) => (
          <TR key={row.id}>
            {showUser && <TD className="font-mono text-text-primary">{row.username}</TD>}
            <TD>
              {row.browser}
              {row.current && (
                <Badge tone="accent" className="ml-2">
                  Cette session
                </Badge>
              )}
            </TD>
            <TD className="font-mono text-[11px]">{row.ipAddress}</TD>
            <TD>{row.authMethod}</TD>
            <TD>{row.lastSeen}</TD>
            <TD className="font-mono text-[11px]">{row.createdAt}</TD>
            <TD className="text-right">
              <Button
                size="sm"
                variant="ghost"
                aria-label={`Révoquer la session ${row.browser} de ${row.username}`}
                disabled={revoking}
                onClick={() => onRevoke(row.id)}
              >
                Révoquer
              </Button>
            </TD>
          </TR>
        ))}
      </TBody>
    </Table>
  )
}
