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
