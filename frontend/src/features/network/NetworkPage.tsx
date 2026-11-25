import { ArrowDownRight, ArrowUpRight, Cable, RefreshCw } from 'lucide-react'
import { Chart } from '../../components/charts/Chart'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardFooter, CardHeader } from '../../components/ui/Card'
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/DataTable'
import { EmptyState } from '../../components/ui/EmptyState'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { PageHeader } from '../../components/ui/PageHeader'
import { SegmentedTabs } from '../../components/ui/SegmentedTabs'
import { Select } from '../../components/ui/Select'
import { formatBytes, formatNumber, formatRate } from '../../lib/format'
import { ranges, useNetworkPage } from './useNetworkPage'

export function NetworkPage() {
  const {
    range,
    setRange,
    setSelectedInterface,
    current,
    interfaces,
    iface,
    history,
    totalRx,
    totalTx,
    currentRxRate,
    currentTxRate,
    totalErrors,
    rxSeries,
    txSeries,
    chartOption,
  } = useNetworkPage()

  return (
    <div className="space-y-4">
      <PageHeader
        title="Réseau"
        breadcrumb={[{ label: 'Système' }, { label: 'Réseau' }]}
        badge={
          <Badge tone={totalErrors > 0 ? 'warning' : 'success'}>
            {totalErrors > 0 ? `${formatNumber(totalErrors)} erreur(s)` : 'Aucune erreur'}
          </Badge>
        }
        description="Interfaces locales, compteurs cumulés et débits mesurés par le collecteur système. Les adresses et compteurs proviennent du noyau Linux."
        actions={
          <Button
            size="sm"
            variant="secondary"
            icon={<RefreshCw className="h-3.5 w-3.5" />}
            onClick={() => {
              void current.refetch()
            }}
          >
            Actualiser
          </Button>
        }
      />

      {current.isError && (
        <ErrorState
          message={current.error.message}
          onRetry={() => {
            void current.refetch()
          }}
        />
      )}
      {current.isLoading && !current.isError && (
        <LoadingState label="Chargement des interfaces réseau…" rows={2} />
      )}

      {!current.isLoading && !current.isError && (
        <>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <Summary
              label="Trafic reçu"
              value={formatBytes(totalRx)}
              hint={`${interfaces.length} interface(s)`}
              tone="text-net"
            />
            <Summary
              label="Trafic envoyé"
              value={formatBytes(totalTx)}
              hint="Compteurs cumulés"
              tone="text-accent"
            />
            <Summary
              label="Débit courant"
              value={formatRate(currentRxRate)}
              hint={`TX ${formatRate(currentTxRate)}`}
              tone="text-text-primary"
            />
            <Summary
              label="Erreurs & pertes"
              value={formatNumber(totalErrors)}
              hint="Cumul sur les interfaces"
              tone={totalErrors > 0 ? 'text-warning' : 'text-success'}
            />
          </div>

          {interfaces.length === 0 ? (
            <EmptyState
              icon={Cable}
              title="Aucune interface mesurée"
              description="Le collecteur système n'a pas publié de compteurs réseau pour cette machine."
            />
          ) : (
            <Card>
              <CardHeader
                title="Interfaces réseau"
                subtitle={`${interfaces.length} interface(s) active(s) ou observée(s)`}
                icon={<Cable className="h-3.5 w-3.5" />}
              />
              <CardBody className="p-0">
                <Table minWidthClassName="min-w-[980px]" className="rounded-none border-0">
                  <THead>
                    <tr>
                      <TH>Interface</TH>
                      <TH>Adresses</TH>
                      <TH>MAC</TH>
                      <TH>RX</TH>
                      <TH>TX</TH>
                      <TH>Débits</TH>
                      <TH>Erreurs</TH>
                      <TH>État</TH>
                    </tr>
                  </THead>
                  <TBody>
                    {interfaces.map((entry) => (
                      <TR key={entry.name}>
                        <TD className="font-mono text-xs text-text-primary">{entry.name}</TD>
                        <TD>
                          <div className="space-y-0.5 font-mono text-xs text-text-secondary">
                            {entry.addresses.length === 0 && (
                              <span className="text-text-muted">—</span>
                            )}
                            {entry.addresses.map((address) => (
                              <span key={address} className="block">
                                {address}
                              </span>
                            ))}
                          </div>
                        </TD>
                        <TD className="font-mono text-xs">{entry.mac || '—'}</TD>
                        <TD className="font-mono text-xs">{formatBytes(entry.rx)}</TD>
                        <TD className="font-mono text-xs">{formatBytes(entry.tx)}</TD>
                        <TD>
                          <div className="space-y-1 text-xs">
                            <span className="flex items-center gap-1 text-text-secondary">
                              <ArrowDownRight className="h-3 w-3 text-net" />{' '}
                              {formatRate(entry.rxRate)}
                            </span>
                            <span className="flex items-center gap-1 text-text-secondary">
                              <ArrowUpRight className="h-3 w-3 text-accent" />{' '}
                              {formatRate(entry.txRate)}
                            </span>
                          </div>
                        </TD>
                        <TD>
                          <Badge tone={entry.errors + entry.drops > 0 ? 'warning' : 'success'}>
                            {formatNumber(entry.errors + entry.drops)}
                          </Badge>
                        </TD>
                        <TD>
                          <Badge
                            tone={entry.rx != null || entry.tx != null ? 'success' : 'neutral'}
                            dot={entry.rx != null || entry.tx != null}
                          >
                            {entry.rx != null || entry.tx != null ? 'Active' : 'Inconnue'}
                          </Badge>
                        </TD>
                      </TR>
                    ))}
                  </TBody>
                </Table>
              </CardBody>
              <CardFooter className="flex flex-wrap items-center justify-between gap-2 text-xs text-text-muted">
                <span>Compteurs cumulés depuis le démarrage de la machine.</span>
                <span>Loopback exclu de la vue synthétique.</span>
              </CardFooter>
            </Card>
          )}

          <Card>
            <CardHeader
              title="Débit par interface"
              subtitle={iface ? `Interface ${iface}` : 'Aucune interface sélectionnée'}
              icon={<ArrowDownRight className="h-3.5 w-3.5" />}
              actions={
                <div className="flex flex-wrap items-center gap-2">
                  <Select
                    aria-label="Choisir une interface réseau"
                    value={iface}
                    onChange={(event) => setSelectedInterface(event.target.value)}
                    className="h-8 max-w-[170px] text-xs"
                  >
                    {interfaces.map((entry) => (
                      <option key={entry.name} value={entry.name}>
                        {entry.name}
                      </option>
                    ))}
                  </Select>
                  <SegmentedTabs
                    ariaLabel="Période de l'historique réseau"
                    value={range}
                    onChange={setRange}
                    options={ranges.map((item) => ({ value: item.value, label: item.label }))}
                  />
                </div>
              }
            />
            <CardBody>
              {history.isError && <ErrorState message={history.error.message} />}
              {!history.isError && rxSeries.length === 0 && txSeries.length === 0 && (
                <p className="py-8 text-center text-sm text-text-muted">
                  Aucun débit historique sur la période. Les débits nécessitent au moins deux
                  collectes consécutives.
                </p>
              )}
              {(rxSeries.length > 0 || txSeries.length > 0) && (
                <Chart option={chartOption} height={300} label="Historique des débits réseau" />
              )}
            </CardBody>
            <CardFooter className="flex flex-wrap items-center justify-between gap-2 text-xs text-text-muted">
              <span>
                {rxSeries.length} point(s) RX · {txSeries.length} point(s) TX
              </span>
              <span>Les débits sont calculés entre deux collectes successives.</span>
            </CardFooter>
          </Card>
        </>
      )}
    </div>
  )
}

function Summary({
  label,
  value,
  hint,
  tone,
}: {
  label: string
  value: string
  hint: string
  tone: string
}) {
  return (
    <div className="rounded-lg border border-line bg-card p-4">
      <p className="text-xs text-text-muted">{label}</p>
      <p className={`mt-1 text-[28px] font-semibold tracking-[-0.03em] ${tone}`}>{value}</p>
      <p className="mt-1 text-xs text-text-muted">{hint}</p>
    </div>
  )
}
