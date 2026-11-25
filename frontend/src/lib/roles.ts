import type { Role } from './api/auth'

const labels: Record<Role, string> = {
  viewer: 'Lecteur',
  operator: 'Opérateur',
  admin: 'Administrateur',
}

/** Roles in increasing order of rights, for select inputs. */
export const roleOptions: Array<{ value: Role; label: string }> = (
  ['viewer', 'operator', 'admin'] as const
).map((value) => ({ value, label: labels[value] }))

export function roleLabel(role: Role): string {
  return labels[role]
}

export function isRole(value: string): value is Role {
  return roleOptions.some((option) => option.value === value)
}

// Display-only mirror of the server policy (SPEC_ADMINISTRATION.md § 3.2):
// it hides actions, the server decides.
export function canManageUsers(role: Role | undefined): boolean {
  return role === 'admin'
}

export function canManageSettings(role: Role | undefined): boolean {
  return role === 'admin'
}

export function canManagePower(role: Role | undefined): boolean {
  return role === 'admin'
}

/** Acknowledge alerts (and silences, phase 4B): operators and administrators. */
export function canOperateAlerts(role: Role | undefined): boolean {
  return role === 'operator' || role === 'admin'
}

/** Create, change and delete alert rules: administrators. */
export function canManageAlerts(role: Role | undefined): boolean {
  return role === 'admin'
}

/** Start, stop, restart and reload the units of the allow list: operators and administrators. */
export function canManageServices(role: Role | undefined): boolean {
  return role === 'operator' || role === 'admin'
}

/** Start a backup: operators and administrators. */
export function canRunBackups(role: Role | undefined): boolean {
  return role === 'operator' || role === 'admin'
}

/** Repository, sources, exclusions and schedule: administrators. */
export function canManageBackups(role: Role | undefined): boolean {
  return role === 'admin'
}

/** Start, stop and restart containers (D-5): operators and administrators. */
export function canManageContainers(role: Role | undefined): boolean {
  return role === 'operator' || role === 'admin'
}
