import type { SegmentedOption } from '../../components/ui/SegmentedTabs'

export type UsersTab = 'accounts' | 'sessions' | 'policy' | 'audit'

export const usersTabs: ReadonlyArray<SegmentedOption<UsersTab>> = [
  { value: 'accounts', label: 'Comptes' },
  { value: 'sessions', label: 'Sessions' },
  { value: 'policy', label: 'Politique' },
  { value: 'audit', label: "Journal d'audit" },
]
