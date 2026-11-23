import type { Role } from './api/auth'
import type { Account } from './api/users'
import { formatDateTime } from './format'
import { roleLabel } from './roles'

export type AccountRow = {
  id: string
  username: string
  displayName: string
  role: Role
  roleLabel: string
  disabled: boolean
  statusLabel: string
  statusTone: 'success' | 'warning' | 'neutral'
  /** Label of the button that toggles the disabled state. */
  toggleLabel: string
  lastLogin: string
  isSelf: boolean
}

function accountStatus(account: Account): Pick<AccountRow, 'statusLabel' | 'statusTone'> {
  if (account.disabled) return { statusLabel: 'Désactivé', statusTone: 'neutral' }
  if (account.mustChangePassword)
    return { statusLabel: 'Mot de passe à changer', statusTone: 'warning' }
  return { statusLabel: 'Actif', statusTone: 'success' }
}

function lastLogin(account: Account): string {
  if (!account.lastLoginAt) return 'Jamais'
  const when = formatDateTime(account.lastLoginAt)
  return account.lastLoginIp ? `${when} · ${account.lastLoginIp}` : when
}

/** Table row of the accounts page; the current account is flagged. */
export function toAccountRow(account: Account, currentUserId: string | undefined): AccountRow {
  return {
    id: account.id,
    username: account.username,
    displayName: account.displayName,
    role: account.role,
    roleLabel: roleLabel(account.role),
    disabled: account.disabled,
    ...accountStatus(account),
    toggleLabel: account.disabled ? 'Réactiver' : 'Désactiver',
    lastLogin: lastLogin(account),
    isSelf: account.id === currentUserId,
  }
}
