import { Copy, ExternalLink, Search, ShieldAlert, ShieldCheck, ShieldQuestion } from 'lucide-react'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/DataTable'
import { EmptyState } from '../../components/ui/EmptyState'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { SegmentedTabs } from '../../components/ui/SegmentedTabs'
import { cn } from '../../lib/cn'
import { formatCount } from '../../lib/format'
import { useVulnerabilitiesPanel, type VulnerabilitiesPanelModel } from './useVulnerabilitiesPanel'

const toneText: Record<string, string> = {
  danger: 'text-danger',
  warning: 'text-warning',
  accent: 'text-accent',
  info: 'text-info',
  neutral: 'text-text-primary',
  success: 'text-success',
}

export function VulnerabilitiesPanel() {
  const page = useVulnerabilitiesPanel()

  return (
    <div className="space-y-4">
      <ScanCard page={page} />
      {page.cards.length > 0 && (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4 xl:grid-cols-7">
          {page.cards.map((card) => (
            <div key={card.key} className="rounded-lg border border-line bg-card p-3">
              <p className="text-xs text-text-muted">{card.label}</p>
              <p
                className={cn(
                  'mt-1 text-[28px] font-semibold tracking-[-0.03em]',
                  card.value > 0 ? toneText[card.tone] : 'text-text-primary',
                )}
              >
                {formatCount(card.value)}
              </p>
            </div>
          ))}
        </div>
      )}
      <FixAllCard page={page} />
      <Findings page={page} />
    </div>
  )
}

function ScanCard({ page }: { page: VulnerabilitiesPanelModel }) {
  const { status } = page
  if (page.summaryState.isLoading) return <LoadingState label="Chargement des analyses…" rows={2} />
  if (page.summaryState.isError)
    return (
      <ErrorState message={page.summaryState.error.message} onRetry={page.summaryState.refetch} />
    )
  if (!status) return null
  return (
    <Card>
      <CardHeader
        title="Vulnérabilités des paquets"
        subtitle="Paquets système de cette machine, analysés par Grype"
        icon={<ShieldAlert className="h-3.5 w-3.5" />}
        actions={
          <Badge tone={status.tone} dot>
            {status.label}
          </Badge>
        }
      />
      <CardBody className="space-y-3">
        {status.failure && (
          <p
            className="rounded-md border border-danger/30 bg-danger/5 px-3 py-2 text-xs text-danger"
            role="alert"
          >
            Dernière analyse en échec le {status.failure}. Les résultats ci-dessous datent de la
            dernière analyse réussie.
          </p>
        )}
        <dl className="grid gap-2 text-xs sm:grid-cols-2 xl:grid-cols-4">
          <Fact label="Dernière analyse réussie" value={status.lastSuccess} />
          <Fact label="Prochaine analyse" value={`${status.next} (toutes les ${page.interval})`} />
          <Fact label="Base de vulnérabilités" value={status.database ?? '—'} />
          <Fact label="Système analysé" value={status.distro ?? '—'} />
        </dl>
        {page.changes && <p className="text-xs text-text-muted">{page.changes}.</p>}
        <p className="text-xs text-text-muted">
          Une vulnérabilité listée est connue pour la version installée d’un paquet ; elle n’est pas
          forcément exploitable sur cette machine. {formatCount(page.fixable)} ont une version
          corrigée disponible.
        </p>
        {page.canScan && (
          <div className="flex flex-wrap items-center gap-2">
            <Button
              size="sm"
              variant="primary"
              loading={page.isStarting}
              disabled={page.scanDisabled}
              onClick={page.startScan}
            >
              Analyser maintenant
            </Button>
            {page.scanError && (
              <span className="text-xs text-danger" role="alert">
                {page.scanError}
              </span>
            )}
          </div>
        )}
      </CardBody>
    </Card>
  )
}

function CommandLine({
  command,
  slot,
  copyLabel,
  copy,
}: {
  command: string
  slot: string
  copyLabel: string
  copy: VulnerabilitiesPanelModel['copy']
}) {
  return (
    <div className="flex items-start gap-2">
      <code className="min-w-0 flex-1 select-all break-all rounded-md border border-line bg-card px-2 py-1.5 text-xs text-text-primary">
        {command}
      </code>
      {copy.available && (
        <Button
          size="sm"
          aria-label={copyLabel}
          icon={<Copy className="h-3.5 w-3.5" />}
          onClick={() => copy.copy(slot, command)}
        >
          Copier
        </Button>
      )}
    </div>
  )
}

function CopyFeedback({ slot, copy }: { slot: string; copy: VulnerabilitiesPanelModel['copy'] }) {
  return (
    <>
      {copy.copiedSlot === slot && (
        <p className="text-xs text-success" role="status">
          Commande copiée
        </p>
      )}
      {copy.errorSlot === slot && (
        <p className="text-xs text-danger" role="alert">
          {copy.error}
        </p>
      )}
    </>
  )
}

function FixAllCard({ page }: { page: VulnerabilitiesPanelModel }) {
  if (page.fixAll.state.isError) {
    return (
      <ErrorState
        title="Commandes de correction indisponibles"
        message={page.fixAll.state.error.message}
        onRetry={page.fixAll.state.refetch}
      />
    )
  }
  if (page.fixAll.commands.length === 0) return null
  return (
    <section aria-label="Tout corriger">
      <Card>
        <CardHeader
          title="Tout corriger"
          subtitle="Le Manager n’exécute rien : lancer la commande sur la machine, en root (ou avec sudo), puis relancer une analyse pour vérifier."
        />
        <CardBody className="space-y-3">
          {page.fixAll.commands.map((entry) => (
            <div key={entry.slot} className="space-y-1">
              <p className="text-xs text-text-secondary">
                {entry.label}
                {entry.packagesLabel && ` · ${entry.packagesLabel}`}
              </p>
              <CommandLine
                command={entry.command}
                slot={entry.slot}
                copyLabel={entry.copyLabel}
                copy={page.copy}
              />
              {entry.note && <p className="text-xs text-text-muted">{entry.note}</p>}
              <p className="text-xs text-text-secondary">{entry.followUp}</p>
              <CopyFeedback slot={entry.slot} copy={page.copy} />
            </div>
          ))}
        </CardBody>
      </Card>
    </section>
  )
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-line/70 bg-surface/50 px-3 py-2">
      <dt className="text-xs text-text-muted">{label}</dt>
      <dd className="mt-1 text-text-primary">{value}</dd>
    </div>
  )
}

function Findings({ page }: { page: VulnerabilitiesPanelModel }) {
  const { filters } = page
  return (
    <Card>
      <CardHeader title="Constats" subtitle="Triés par sévérité puis par date d’apparition" />
      <CardBody className="space-y-3">
        <div className="flex flex-wrap items-center gap-2">
          <SegmentedTabs
            options={filters.stateOptions}
            value={filters.state}
            onChange={filters.setState}
            ariaLabel="Constats affichés"
          />
          <div className="flex flex-wrap gap-1" role="group" aria-label="Sévérités">
            {filters.severityOptions.map((option) => (
              <Button
                key={option.value}
                size="sm"
                variant={option.pressed ? 'primary' : 'secondary'}
                aria-pressed={option.pressed}
                onClick={() => filters.toggleSeverity(option.value)}
              >
                {option.label}
              </Button>
            ))}
          </div>
          <label className="flex items-center gap-1.5 text-xs text-text-secondary">
            <input
              type="checkbox"
              checked={filters.fixableOnly}
              onChange={(event) => filters.setFixableOnly(event.target.checked)}
            />
            Corrigeables seulement
          </label>
          <form
            className="flex items-center gap-1"
            role="search"
            onSubmit={(event) => {
              event.preventDefault()
              filters.applySearch()
            }}
          >
            <label htmlFor="vulnerabilitySearch" className="sr-only">
              Rechercher un identifiant ou un paquet
            </label>
            <input
              id="vulnerabilitySearch"
              value={filters.searchInput}
              onChange={(event) => filters.setSearchInput(event.target.value)}
              maxLength={100}
              placeholder="CVE ou paquet"
              className="h-8 w-44 rounded-md border border-line bg-surface px-2 text-xs text-text-primary outline-none focus:border-accent"
            />
            <Button
              size="sm"
              type="submit"
              aria-label="Rechercher"
              icon={<Search className="h-3.5 w-3.5" />}
            />
          </form>
        </div>

        {page.findingsState.isLoading && <LoadingState label="Chargement des constats…" rows={3} />}
        {page.findingsState.isError && (
          <ErrorState
            message={page.findingsState.error.message}
            onRetry={page.findingsState.refetch}
          />
        )}
        {!page.findingsState.isLoading && !page.findingsState.isError && page.rows.length === 0 && (
          <EmptyState
            icon={page.status?.kind === 'never' ? ShieldQuestion : ShieldCheck}
            title={page.status?.kind === 'never' ? 'Aucune analyse encore' : 'Aucune vulnérabilité'}
            description={
              page.status?.kind === 'never'
                ? 'La première analyse a lieu quelques minutes après le démarrage du Manager.'
                : 'Aucun constat ne correspond à ces filtres.'
            }
          />
        )}
        {page.rows.length > 0 && (
          <>
            <Table minWidthClassName="min-w-[820px]">
              <THead>
                <TR>
                  <TH>Vulnérabilité</TH>
                  <TH>Sévérité</TH>
                  <TH>Paquet</TH>
                  <TH>Installée</TH>
                  <TH>Corrigée</TH>
                  <TH>État</TH>
                  <TH>{filters.state === 'present' ? 'Détectée' : 'Disparue'}</TH>
                </TR>
              </THead>
              <TBody>
                {page.rows.map((row) => (
                  <TR key={row.id} className={cn(page.selectedID === row.id && 'bg-elevated/40')}>
                    <TD>
                      <button
                        type="button"
                        className="font-mono text-xs text-accent hover:underline"
                        aria-expanded={page.selectedID === row.id}
                        onClick={() => page.select(row.id)}
                      >
                        {row.vulnerabilityId}
                      </button>
                    </TD>
                    <TD>
                      <Badge tone={row.severity.tone} dot>
                        {row.severity.label}
                      </Badge>
                    </TD>
                    <TD className="text-text-primary">{row.package}</TD>
                    <TD className="font-mono text-xs">{row.version}</TD>
                    <TD className="font-mono text-xs">{row.fixedVersion}</TD>
                    <TD>
                      <Badge tone={row.fixState.tone}>{row.fixState.label}</Badge>
                    </TD>
                    <TD className="whitespace-nowrap text-xs">{row.resolved ?? row.firstSeen}</TD>
                  </TR>
                ))}
              </TBody>
            </Table>
            <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-text-secondary">
              <span>
                {page.page.from}–{page.page.to} sur {formatCount(page.total)}
              </span>
              <div className="flex gap-1">
                <Button size="sm" disabled={!page.page.hasPrevious} onClick={page.page.previous}>
                  Précédents
                </Button>
                <Button size="sm" disabled={!page.page.hasNext} onClick={page.page.next}>
                  Suivants
                </Button>
              </div>
            </div>
          </>
        )}
        {page.detail && <FindingDetailCard page={page} />}
      </CardBody>
    </Card>
  )
}

function FindingDetailCard({ page }: { page: VulnerabilitiesPanelModel }) {
  const detail = page.detail
  if (!detail) return null
  return (
    <section
      aria-label={`Détail de ${detail.vulnerabilityId}`}
      className="space-y-2 rounded-md border border-line bg-surface/50 p-3 text-xs"
    >
      <div className="flex flex-wrap items-center gap-2">
        <h3 className="font-mono text-sm text-text-primary">{detail.vulnerabilityId}</h3>
        <Badge tone={detail.severity.tone} dot>
          {detail.severity.label}
        </Badge>
        <Badge tone={detail.fixState.tone}>{detail.fixState.label}</Badge>
      </div>
      <p className="whitespace-pre-line text-text-secondary">{detail.description}</p>
      <dl className="grid gap-1 sm:grid-cols-2">
        <div>
          <dt className="inline text-text-muted">Paquet : </dt>
          <dd className="inline font-mono">
            {detail.package} {detail.version}
          </dd>
        </div>
        <div>
          <dt className="inline text-text-muted">Versions corrigées : </dt>
          <dd className="inline font-mono">{detail.fixedVersions}</dd>
        </div>
        <div>
          <dt className="inline text-text-muted">Première détection : </dt>
          <dd className="inline">{detail.firstSeen}</dd>
        </div>
        <div>
          <dt className="inline text-text-muted">Dernière détection : </dt>
          <dd className="inline">{detail.lastSeen}</dd>
        </div>
      </dl>
      <section aria-label="Comment la corriger" className="space-y-1.5">
        <h4 className="font-medium text-text-primary">Comment la corriger</h4>
        <p className="text-text-secondary">{detail.remediation.summary}</p>
        {detail.remediation.command && (
          <CommandLine
            command={detail.remediation.command}
            slot={detail.slot}
            copyLabel={detail.copyLabel}
            copy={page.copy}
          />
        )}
        {detail.remediation.target && (
          <p className="text-text-secondary">Version visée : {detail.remediation.target}</p>
        )}
        {detail.remediation.followUp && (
          <p className="text-text-secondary">{detail.remediation.followUp}</p>
        )}
        {detail.remediation.command && (
          <p className="text-xs text-text-muted">
            Le Manager n’exécute rien : lancer la commande sur la machine, en root (ou avec sudo).
          </p>
        )}
        {detail.remediation.command && <CopyFeedback slot={detail.slot} copy={page.copy} />}
      </section>
      {detail.advisory && (
        <a
          href={detail.advisory.url}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-1 text-accent hover:underline"
        >
          {detail.advisory.label} <ExternalLink className="h-3 w-3" aria-hidden="true" />
        </a>
      )}
      {detail.references.length > 0 && (
        <details>
          <summary className="cursor-pointer text-text-muted">
            Références ({detail.references.length})
          </summary>
          <ul className="mt-1 space-y-0.5 break-all text-xs text-text-secondary">
            {detail.references.map((reference) => (
              <li key={reference}>{reference}</li>
            ))}
          </ul>
        </details>
      )}
    </section>
  )
}
