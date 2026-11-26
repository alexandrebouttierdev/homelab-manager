import { useState } from 'react'
import { PageHeader } from '../../components/ui/PageHeader'
import { SegmentedTabs } from '../../components/ui/SegmentedTabs'
import { AccountsPanel } from './AccountsPanel'
import { AuditPanel } from './AuditPanel'
import { SecurityPolicyPanel } from './SecurityPolicyPanel'
import { SessionsPanel } from './SessionsPanel'
import { usersTabs, type UsersTab } from './usersTabs'

export function UsersPage() {
  const [tab, setTab] = useState<UsersTab>('accounts')

  return (
    <div className="space-y-4">
      <PageHeader
        title="Utilisateurs & accès"
        breadcrumb={[{ label: 'Paramètres' }, { label: 'Utilisateurs & accès' }]}
        description="Comptes de MonoOps uniquement : aucun compte Linux de la machine n'est lu ni modifié."
      />
      <SegmentedTabs options={usersTabs} value={tab} onChange={setTab} ariaLabel="Sections" />
      {tab === 'accounts' && <AccountsPanel />}
      {tab === 'sessions' && <SessionsPanel />}
      {tab === 'policy' && <SecurityPolicyPanel />}
      {tab === 'audit' && <AuditPanel />}
    </div>
  )
}
